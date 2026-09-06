package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// digester は digesters.yaml が宣言する集計 1 件。
// ベンチ後の集計は「入力があるか確かめる → 外部ツールがあるか確かめる →
// タイムアウト付きで実行する → 失敗なら理由を残して既定値で埋める」という
// 形が共通なので、違うのは宣言に書いた入力・コマンド・既定値だけになる。
type digestRunner struct {
	cfg     *DigestConfig
	runDir  string
	rawDir  string
	sshUser string
	sshOpts []string
	roles   map[string][]string
	vars    map[string]string
	dryRun  bool
	// bench.log が preflight 失敗を示しているか。skip_if_bench_failed の判定に使う。
	benchFailed bool
	loadWindow  LoadWindow
	// fetchFn is used by focused tests; production fetches through rsyncFrom.
	fetchFn     func(host, remote, local string) error
	sshScriptFn func(host, script string) (string, string, error)
}

func runDigest(args []string) error {
	fs := flag.NewFlagSet("digest", flag.ExitOnError)
	config := fs.String("config", "tools/measurectl/digesters.yaml", "集計の宣言")
	runDir := fs.String("run-dir", "", "runs/<RUN_ID> (必須)")
	rawDir := fs.String("raw-dir", "raw", "走行をまたいで上書きする生ログの置き場")
	sshUser := fs.String("ssh-user", "ubuntu", "SSH ユーザー")
	sshOpts := fs.String("ssh-opts", "", "ssh へ渡す追加オプション (空白区切り)")
	only := fs.String("only", "", "この集計だけを対象にする (カンマ区切り)")
	skipFetch := fs.Bool("skip-fetch", false, "生ログの回収を飛ばし、手元のログだけで集計し直す")
	dryRun := fs.Bool("dry-run", false, "実行せず、流すコマンドだけを表示する")
	roles := keyValues{}
	fs.Var(roles, "role", "役割名=ホスト1,ホスト2 (sources の role が指す先)")
	vars := keyValue{}
	fs.Var(vars, "var", "digesters.yaml の {var:NAME} を埋める値")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runDir == "" {
		return errors.New("-run-dir は必須です")
	}

	cfg, err := loadDigestConfig(*config)
	if err != nil {
		return err
	}
	if *only != "" {
		cfg.Digesters = filterDigesters(cfg.Digesters, strings.Split(*only, ","))
		if len(cfg.Digesters) == 0 {
			return errors.New("-only にマッチする集計がありません")
		}
	} else {
		var enabled []Digester
		for _, d := range cfg.Digesters {
			if d.enabledByDefault() {
				enabled = append(enabled, d)
			}
		}
		cfg.Digesters = enabled
	}

	r := &digestRunner{
		cfg:     cfg,
		runDir:  *runDir,
		rawDir:  *rawDir,
		sshUser: *sshUser,
		sshOpts: strings.Fields(*sshOpts),
		roles:   roles,
		vars:    vars,
		dryRun:  *dryRun,
	}
	r.benchFailed = benchFailed(filepath.Join(*runDir, "bench.log"))
	benchLog, _ := os.ReadFile(filepath.Join(*runDir, "bench.log"))
	r.loadWindow = resolveLoadWindow(benchLog)

	if !*dryRun {
		if err := os.MkdirAll(*runDir, 0o755); err != nil {
			return err
		}
	}

	// 1. 生ログの回収。集計はこれを待つので、ここだけは先に揃える。
	if !*skipFetch {
		if err := r.fetchAll(); err != nil {
			// 1 つの source が取れなくても、取れた入力での集計は進めたい。
			fmt.Fprintf(os.Stderr, "measurectl: 生ログの回収に失敗しました: %v\n", err)
		}
	}

	// 2. 集計。互いに独立なので並列に流す。
	digestErr := r.digestAll()

	// 3. 集計が読み終わってから生ログを畳む。
	if err := r.compressAll(); err != nil {
		fmt.Fprintf(os.Stderr, "measurectl: 生ログの圧縮に失敗しました: %v\n", err)
	}
	return digestErr
}

func filterDigesters(all []Digester, names []string) []Digester {
	want := map[string]bool{}
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	var out []Digester
	for _, d := range all {
		if want[d.Name] {
			out = append(out, d)
		}
	}
	return out
}

// ---- 生ログの回収 ------------------------------------------------------

func (r *digestRunner) fetchAll() error {
	type job struct {
		src  Source
		host string
	}
	var jobs []job
	for _, s := range r.cfg.Sources {
		hosts, ok := r.roles[s.Role]
		if !ok {
			return fmt.Errorf("source %q が指す役割 %q のホストが -role で渡されていません", s.Name, s.Role)
		}
		for _, h := range hosts {
			jobs = append(jobs, job{src: s, host: h})
		}
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			local := r.expand(j.src.Local, j.host)
			if err := r.fetchSource(j.src, j.host); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("[%s] %s: %w", j.host, j.src.Name, err))
				r.markSourceFetchFailure(j.src, j.host, err)
				mu.Unlock()
				return
			}
			if !r.dryRun {
				fmt.Printf("[%s] %s 保存\n", j.host, local)
			}
		}(j)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (r *digestRunner) fetchSource(src Source, host string) error {
	local := r.expand(src.Local, host)
	if r.dryRun {
		return r.rsyncFrom(host, src.Remote, local)
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return err
	}
	for _, stale := range []string{local, local + ".zst"} {
		if err := os.Remove(stale); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stale raw を削除できません: %w", err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(local), "."+filepath.Base(local)+".fetch-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	fetch := r.rsyncFrom
	if r.fetchFn != nil {
		fetch = r.fetchFn
	}
	if err := fetch(host, src.Remote, tmpPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, local); err != nil {
		return fmt.Errorf("raw を atomic rename できません: %w", err)
	}
	return nil
}

func (r *digestRunner) markSourceFetchFailure(src Source, host string, err error) {
	if r.dryRun {
		return
	}
	reason := fmt.Sprintf("%s source fetch failed on %s: %v", src.Name, host, err)
	for _, d := range r.cfg.Digesters {
		if d.Source != src.Name {
			continue
		}
		r.appendStderr(d, reason)
		for _, out := range d.Outputs {
			path := filepath.Join(r.runDir, out.File)
			if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
				_ = os.WriteFile(path, nil, 0o644)
			}
		}
	}
}

// ---- 集計 --------------------------------------------------------------

func (r *digestRunner) digestAll() error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, d := range r.cfg.Digesters {
		wg.Add(1)
		go func(d Digester) {
			defer wg.Done()
			if err := r.digest(d); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", d.Name, err))
				mu.Unlock()
			}
		}(d)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (r *digestRunner) digest(d Digester) error {
	if d.Remote != nil {
		return r.digestRemote(d)
	}

	src, ok := r.cfg.source(d.Source)
	if !ok {
		return fmt.Errorf("source %q が宣言にありません", d.Source)
	}

	// スキップ条件を順に見る。どれかに当たれば、理由を stderr に残して
	// on_skip の既定値でファイルを埋める (解析側が中身の有無で分岐しなくて済む)。
	if reason := r.skipReason(d, src); reason != "" {
		return r.skip(d, reason)
	}

	inputs, err := r.inputsFor(src)
	if err != nil {
		return err
	}

	// 出力どうしは同じ入力を独立に舐めるだけなので並列に流す。
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, out := range d.Outputs {
		wg.Add(1)
		go func(out Output) {
			defer wg.Done()
			if err := r.runOutput(d, out, inputs); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(out)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// digestRemote はリモートでスクリプトを流し、その標準出力を集計として保存する。
// スクリプトが非 0 で終われば (performance_schema が無効なときなど) スキップ扱いにし、
// 標準エラーの中身をそのまま理由にする。
func (r *digestRunner) digestRemote(d Digester) error {
	if (strings.Contains(d.Remote.Script, "{load_start}") || strings.Contains(d.Remote.Script, "{load_end}")) && r.loadWindow.Status != "ok" {
		return fmt.Errorf("%s requires a valid benchmark load window: %s", d.Name, r.loadWindow.Reason)
	}
	hosts, ok := r.roles[d.Remote.Role]
	if !ok || len(hosts) == 0 {
		return fmt.Errorf("remote が指す役割 %q のホストが -role で渡されていません", d.Remote.Role)
	}
	if !d.Remote.PerHost {
		hosts = hosts[:1]
	}
	return parallelIndex(len(hosts), func(index int) error {
		host := hosts[index]
		script := r.expand(d.Remote.Script, host)
		if missing := unresolved(script); len(missing) > 0 {
			return fmt.Errorf("[%s] remote script has unresolved placeholders: %s", host, strings.Join(missing, " "))
		}
		if r.dryRun {
			fmt.Printf("[dry-run] ssh %s sh -s <<'EOF'\n%sEOF\n", host, script)
			return nil
		}
		sshScript := r.sshScript
		if r.sshScriptFn != nil {
			sshScript = r.sshScriptFn
		}
		out, stderr, err := sshScript(host, script)
		if err != nil {
			reason := strings.TrimSpace(stderr)
			if reason == "" {
				reason = err.Error()
			}
			return r.skipRemoteHost(d, host, fmt.Sprintf("[%s] %s skipped: %s", host, d.Name, reason))
		}
		for _, o := range d.Outputs {
			path := filepath.Join(r.runDir, r.expand(o.File, host))
			body := out
			if o.Header != "" {
				body = unescape(o.Header) + "\n" + body
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Printf("%s 集計（%d 行）\n", path, strings.Count(strings.TrimSpace(out), "\n")+1)
		}
		return nil
	})
}

func (r *digestRunner) skipRemoteHost(d Digester, host, reason string) error {
	r.appendStderr(d, reason)
	for _, output := range d.Outputs {
		if output.OnSkip == "" {
			continue
		}
		path := filepath.Join(r.runDir, r.expand(output.File, host))
		if err := os.WriteFile(path, []byte(unescape(output.OnSkip)+"\n"), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("%s の集計をスキップ（%s）\n", d.Name, reason)
	return nil
}

func parallelIndex(count int, fn func(int) error) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			if err := fn(index); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(index)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// skipReason は集計を飛ばす理由を返す。飛ばさないなら空文字。
func (r *digestRunner) skipReason(d Digester, src Source) string {
	inputs, err := r.inputsFor(src)
	if err != nil || len(inputs) == 0 {
		return fmt.Sprintf("%s skipped: %s not collected", d.Name, src.Name)
	}
	if d.SkipIfBenchFailed && r.benchFailed {
		return fmt.Sprintf("%s skipped: benchmark failed during preflight", d.Name)
	}
	// 先頭コマンドが PATH に無ければ飛ばす。宣言に書かなくても、実行しようと
	// している当のコマンドから判定できる。
	for _, out := range d.Outputs {
		fields := strings.Fields(out.Run)
		if len(fields) == 0 {
			continue
		}
		bin := fields[0]
		if strings.ContainsRune(bin, filepath.Separator) {
			continue // リポジトリ内のツールは PATH ではなく相対パスで指す
		}
		if _, err := exec.LookPath(bin); err != nil {
			reason := fmt.Sprintf("%s command not found in PATH", bin)
			if d.InstallHint != "" {
				reason += " (" + d.InstallHint + ")"
			}
			return reason
		}
	}
	return ""
}

func (r *digestRunner) skip(d Digester, reason string) error {
	if r.dryRun {
		fmt.Printf("[dry-run] skip %s: %s\n", d.Name, reason)
		return nil
	}
	if d.Stderr != "" {
		if err := os.WriteFile(filepath.Join(r.runDir, d.Stderr), []byte(reason+"\n"), 0o644); err != nil {
			return err
		}
	}
	for _, out := range d.Outputs {
		if out.OnSkip == "" {
			continue
		}
		path := filepath.Join(r.runDir, out.File)
		if err := os.WriteFile(path, []byte(unescape(out.OnSkip)+"\n"), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("%s の集計をスキップ（%s）\n", d.Name, reason)
	return nil
}

func (r *digestRunner) runOutput(d Digester, out Output, inputs []string) error {
	path := filepath.Join(r.runDir, out.File)
	argv := r.buildArgv(out.Run, inputs)
	if len(argv) == 0 {
		return fmt.Errorf("%s: run が空です", out.File)
	}

	if r.dryRun {
		if d.Stdin != "" {
			fmt.Printf("[dry-run] %s %s | %s > %s\n", d.Stdin, strings.Join(inputs, " "), strings.Join(argv, " "), path)
		} else {
			fmt.Printf("[dry-run] %s > %s\n", strings.Join(argv, " "), path)
		}
		return nil
	}

	ctx := context.Background()
	if d.Timeout != "" {
		timeout, err := time.ParseDuration(d.Timeout)
		if err != nil {
			return fmt.Errorf("timeout を解釈できません: %w", err)
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	err := r.exec(ctx, d, argv, inputs, path)
	if err == nil {
		fmt.Printf("%s 集計\n", path)
		return nil
	}

	// timeout は Go 側で見るので、gtimeout の有無で分岐する必要はない。
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("timed out after %s", d.Timeout)
	}
	r.appendStderr(d, fmt.Sprintf("%s failed: %v", out.File, err))

	// on_error があれば、その内容で埋めて続行する。ダッシュボードのように
	// 「壊れた中身より空のほうが扱いやすい」出力のための逃げ道。
	if out.OnError != "" {
		if werr := os.WriteFile(path, []byte(unescape(out.OnError)), 0o644); werr != nil {
			return werr
		}
		fmt.Fprintf(os.Stderr, "%s の集計に失敗しました (%v)。既定値で埋めます\n", path, err)
		return nil
	}
	return fmt.Errorf("%s: %w", out.File, err)
}

// exec は 1 出力ぶんの実行。stdin が宣言されていれば、そのコマンドで入力を
// 読んで集計コマンドの標準入力へつなぐ。
func (r *digestRunner) exec(ctx context.Context, d Digester, argv, inputs []string, path string) error {
	// 集計コマンドが途中で落ちると、入力を流し込んでいる側は書き込み先を失って
	// 止まったままになる。この呼び出し専用の context を挟んで、抜けるときに
	// 確実に殺す。
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)

	var reader *exec.Cmd
	var closeReader func() error
	if d.Stdin != "" {
		// A consumer can reject the first input row while the producer is still
		// writing a large log. Bound os/exec's internal stdin-copy wait so Run
		// returns and the producer can be cancelled below.
		cmd.WaitDelay = time.Second
		reader = exec.CommandContext(ctx, d.Stdin, inputs...)
		pipe, err := reader.StdoutPipe()
		if err != nil {
			return err
		}
		cmd.Stdin = pipe
		closeReader = pipe.Close
		if err := reader.Start(); err != nil {
			return err
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd.Stdout = f

	var stderr strings.Builder
	cmd.Stderr = &stderr
	err = cmd.Run()
	// If the consumer exits before reading all input, close the parent's pipe
	// copy before waiting so a producer blocked on a full pipe gets EPIPE.
	if closeReader != nil {
		_ = closeReader()
	}
	if reader != nil {
		// Preserve a successful consumer result until the producer has reported
		// its own status. Cancelling first races with a producer that already
		// closed stdout but has not exited yet, turning a complete artifact into
		// a false "context canceled" failure.
		if err != nil {
			cancel()
		}
		if readerErr := reader.Wait(); err == nil && readerErr != nil {
			err = readerErr
		}
	}
	cancel()
	if s := strings.TrimSpace(stderr.String()); s != "" {
		r.appendStderr(d, s)
	}
	return err
}

// buildArgv はコマンド文字列を argv へ落とす。シェルを経由しないので、
// 引数にクォートやリダイレクトは書けない (宣言側でそれが要らない形にしてある)。
func (r *digestRunner) buildArgv(run string, inputs []string) []string {
	var argv []string
	for _, token := range strings.Fields(run) {
		if token == "{input}" {
			argv = append(argv, inputs...)
			continue
		}
		argv = append(argv, r.expand(token, ""))
	}
	return argv
}

// ---- 生ログの圧縮 ------------------------------------------------------

func (r *digestRunner) compressAll() error {
	var errs []error
	for _, s := range r.cfg.Sources {
		if !s.Compress {
			continue
		}
		// Empty logs still need a compressed artifact for every declared host.
		files, err := filepath.Glob(r.expand(s.Local, "*"))
		if err != nil {
			continue
		}
		for _, f := range files {
			if strings.HasSuffix(f, ".zst") {
				continue
			}
			if r.dryRun {
				fmt.Printf("[dry-run] zstd -q -3 -T0 --rm %s\n", f)
				continue
			}
			before := fileSize(f)
			if out, err := exec.Command("zstd", "-q", "-3", "-T0", "--rm", f).CombinedOutput(); err != nil {
				errs = append(errs, fmt.Errorf("zstd %s: %w: %s", f, err, strings.TrimSpace(string(out))))
				continue
			}
			after := fileSize(f + ".zst")
			fmt.Printf("%s.zst 圧縮 (%dMB -> %dMB)\n", filepath.Base(f), before>>20, after>>20)
		}
	}
	return errors.Join(errs...)
}

// ---- 小物 --------------------------------------------------------------

// inputsFor は source のローカルパスを展開して、実在するファイルを返す。
// {host} を含むパスは glob として扱う (ホストごとに 1 本ずつ落ちてくる)。
func (r *digestRunner) inputsFor(s Source) ([]string, error) {
	pattern := r.expand(s.Local, "*")
	// 圧縮済みの回も拾う。zstdcat は非圧縮ファイルもそのまま通す。
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	zst, err := filepath.Glob(pattern + ".zst")
	if err != nil {
		return nil, err
	}

	// 同じログの非圧縮版と圧縮版が両方あるとき (前の走行の .zst が残ったまま
	// 新しい .log を回収したとき) は、非圧縮のほうだけを使う。両方渡すと
	// 同じログを二重に集計してしまう。
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		if fileSize(m) > 0 {
			seen[m] = true
			out = append(out, m)
		}
	}
	for _, m := range zst {
		if seen[strings.TrimSuffix(m, ".zst")] {
			continue
		}
		if fileSize(m) > 0 {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *digestRunner) expand(s, host string) string {
	expanded := strings.NewReplacer(
		"{run_dir}", r.runDir,
		"{raw_dir}", r.rawDir,
		"{host}", host,
		"{load_start}", r.loadWindow.StartedAt,
		"{load_end}", r.loadWindow.EndedAt,
	).Replace(s)
	return placeholderRe.ReplaceAllStringFunc(expanded, func(key string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(key, "{var:"), "}")
		if value, ok := r.vars[name]; ok {
			return value
		}
		return key
	})
}

func (r *digestRunner) appendStderr(d Digester, msg string) {
	if d.Stderr == "" || r.dryRun {
		return
	}
	path := filepath.Join(r.runDir, d.Stderr)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, msg)
}

// sshScript はスクリプトを stdin から流し、標準出力と標準エラーを返す。
// リモートで実行する内容を引数に埋め込まないので、シェルのクォートを
// 二重三重にエスケープせずに済む。
func (r *digestRunner) sshScript(host, script string) (string, string, error) {
	args := append([]string{"-o", "RequestTTY=no", "-o", "LogLevel=ERROR"}, r.sshOpts...)
	args = append(args, r.sshUser+"@"+host, "sh -s")

	cmd := exec.Command("ssh", args...)
	cmd.Stdin = strings.NewReader(script)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (r *digestRunner) rsyncFrom(host, remote, local string) error {
	if !r.dryRun {
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			return err
		}
	}
	sshBase := append([]string{"-o", "RequestTTY=no", "-o", "LogLevel=ERROR"}, r.sshOpts...)
	args := []string{"-az", "--rsync-path=sudo rsync", "-e", "ssh " + strings.Join(sshBase, " "),
		r.sshUser + "@" + host + ":" + remote, local}
	if r.dryRun {
		fmt.Printf("[dry-run] rsync %s\n", strings.Join(args, " "))
		return nil
	}
	if out, err := exec.Command("rsync", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("rsync: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// benchFailed は bench.log が preflight 失敗を示しているか。
// ファイルが無い / 読めない場合は「失敗とは判断しない」。
func benchFailed(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return passFalseRe.Match(body)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// unescape は宣言に書いた \t / \n を実文字へ直す。YAML の二重引用符でも
// 書けるが、宣言側で意識せずに済むようこちらでも受ける。
func unescape(s string) string {
	return strings.NewReplacer(`\t`, "\t", `\n`, "\n").Replace(s)
}
