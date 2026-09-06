package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// runner は 1 回の collect コマンドの実行文脈。collectors.yaml の宣言と、
// Taskfile から渡された役割・変数・SSH 設定を突き合わせて実際の操作へ落とす。
type runner struct {
	cfg     *Config
	runID   string
	runDir  string
	sshUser string
	sshOpts []string
	roles   map[string][]string
	vars    map[string]string
	dryRun  bool
	// -only で絞られた対象名。nil なら全部。collectors は cfg 側を絞るので、
	// prepare / oneshots の絞り込みにだけ使う。
	only map[string]bool
}

// target は「どの collector をどのホストで扱うか」の 1 単位。
// collector × ホストの全組み合わせを並列に流す。
type target struct {
	c    Collector
	host string
}

func runCollect(args []string) error {
	if len(args) == 0 {
		return errors.New("collect の後に start / stop / check-clean / sweep を指定してください")
	}
	action := args[0]

	fs := flag.NewFlagSet("collect "+action, flag.ExitOnError)
	config := fs.String("config", "tools/measurectl/collectors.yaml", "collector 定義")
	runID := fs.String("run-id", "", "走行 ID (start / stop で必須)")
	runDir := fs.String("run-dir", "", "回収先 runs/<RUN_ID> (stop で必須)")
	sshUser := fs.String("ssh-user", "ubuntu", "SSH ユーザー")
	sshOpts := fs.String("ssh-opts", "", "ssh へ渡す追加オプション (空白区切り)")
	only := fs.String("only", "", "この collector だけを対象にする (カンマ区切り)")
	group := fs.String("group", "", "enabled oneshots in this declaration group (oneshot/ready only)")
	include := fs.String("include", "", "既定無効の collector を追加で有効にする (カンマ区切り)")
	noCollectors := fs.Bool("no-collectors", false, "prepare/digestは維持し、常駐collectorだけを無効にする")
	dryRun := fs.Bool("dry-run", false, "実行せず、流すコマンドだけを表示する")
	roles := keyValues{}
	fs.Var(roles, "role", "役割名=ホスト1,ホスト2 (collectors.yaml の hosts が指す先)")
	vars := keyValue{}
	fs.Var(vars, "var", "collectors.yaml の {var:NAME} を埋める値")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := loadConfig(*config)
	if err != nil {
		return err
	}
	onlyNames := parseNames(*only)
	includeNames := parseNames(*include)
	if *group != "" {
		if (action != "oneshot" && action != "ready") || onlyNames != nil || includeNames != nil || *noCollectors {
			return errors.New("-group requires oneshot/ready and cannot be combined with -only, -include or -no-collectors")
		}
		cfg.Oneshots, err = enabledOneshotsInGroup(cfg.Oneshots, *group)
		if err != nil {
			return err
		}
	}
	if *noCollectors && (onlyNames != nil || includeNames != nil) {
		return errors.New("-no-collectors は -only / -include と同時に指定できません")
	}
	allCollectorAction := action == "sweep" || action == "check-clean"
	if !allCollectorAction {
		if *noCollectors {
			cfg.Collectors = nil
		} else if onlyNames != nil {
			switch action {
			case "start", "stop":
				cfg.Collectors, err = filterCollectors(cfg.Collectors, onlyNames)
			case "prepare":
				err = validateOnlyNames("prepare", onlyNames, prepareNames(cfg.Prepare))
			case "oneshot", "ready":
				err = validateOnlyNames("oneshot", onlyNames, oneshotNames(cfg.Oneshots))
			}
			if err != nil {
				return err
			}
		} else {
			if action == "oneshot" || action == "ready" {
				cfg.Oneshots, err = enabledOneshots(cfg.Oneshots, includeNames)
			} else {
				cfg.Collectors, err = enabledCollectors(cfg.Collectors, includeNames)
			}
			if err != nil {
				return err
			}
		}
	}

	r := &runner{
		only:    onlyNames,
		cfg:     cfg,
		runID:   *runID,
		runDir:  *runDir,
		sshUser: *sshUser,
		sshOpts: strings.Fields(*sshOpts),
		roles:   roles,
		vars:    vars,
		dryRun:  *dryRun,
	}

	switch action {
	case "prepare":
		return r.prepareAll()
	case "start":
		if r.runID == "" {
			return errors.New("-run-id は必須です")
		}
		return r.each(r.start)
	case "stop":
		if r.runID == "" || r.runDir == "" {
			return errors.New("-run-id と -run-dir は必須です")
		}
		return r.each(r.stop)
	case "oneshot":
		if r.runDir == "" {
			return errors.New("-run-dir は必須です")
		}
		return r.oneshotAll()
	case "ready":
		if r.runID == "" {
			return errors.New("-run-id is required for readiness")
		}
		return r.readyAll()
	case "check-clean":
		return r.checkCleanAll()
	case "sweep":
		return r.sweepAll()
	default:
		return fmt.Errorf("不明な操作: %s (prepare / start / stop / oneshot / check-clean / sweep)", action)
	}
}

func (r *runner) readyAll() error {
	type job struct{ script, host string }
	var jobs []job
	for _, o := range r.cfg.Oneshots {
		if r.only != nil && !r.only[o.Name] {
			continue
		}
		if o.Ready == "" {
			continue
		}
		if len(r.roles[o.Hosts]) == 0 {
			return fmt.Errorf("oneshot %s has no hosts", o.Name)
		}
		for _, host := range r.roles[o.Hosts] {
			exp := expander{host: host, runID: r.runID, vars: r.vars}
			script := exp.expand(o.Ready)
			if missing := unresolved(script); len(missing) > 0 {
				return fmt.Errorf("unresolved readiness placeholders: %v", missing)
			}
			jobs = append(jobs, job{script, host})
		}
	}
	// Snapshot-only or explicitly disabled groups need no readiness gate.
	return parallel(jobs, func(j job) error { return r.ssh(j.host, j.script) })
}

func enabledOneshotsInGroup(all []Oneshot, group string) ([]Oneshot, error) {
	var selected []Oneshot
	found := false
	for _, o := range all {
		if o.Group == group {
			found = true
			if o.enabledByDefault() {
				selected = append(selected, o)
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("unknown oneshot group %q", group)
	}
	return selected, nil
}

func parseNames(value string) map[string]bool {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	names := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names[name] = true
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

func enabledCollectors(collectors []Collector, include map[string]bool) ([]Collector, error) {
	known := make(map[string]bool, len(collectors))
	for _, collector := range collectors {
		known[collector.Name] = true
	}
	for name := range include {
		if !known[name] {
			return nil, fmt.Errorf("-include に一致する collector %q がありません", name)
		}
	}

	selected := make([]Collector, 0, len(collectors))
	for _, collector := range collectors {
		if collector.enabledByDefault() || include[collector.Name] {
			selected = append(selected, collector)
		}
	}
	return selected, nil
}

func enabledOneshots(oneshots []Oneshot, include map[string]bool) ([]Oneshot, error) {
	known := oneshotNames(oneshots)
	for name := range include {
		if !known[name] {
			return nil, fmt.Errorf("-include に一致する oneshot %q がありません", name)
		}
	}
	selected := make([]Oneshot, 0, len(oneshots))
	for _, oneshot := range oneshots {
		if oneshot.enabledByDefault() || include[oneshot.Name] {
			selected = append(selected, oneshot)
		}
	}
	return selected, nil
}

// prepareAll は走行前の下ごしらえを対象ホストへ並列に流す。
func (r *runner) prepareAll() error {
	type job struct {
		p    Prepare
		host string
	}
	var jobs []job
	for _, p := range r.cfg.Prepare {
		if r.only != nil && !r.only[p.Name] {
			continue
		}
		hosts, ok := r.roles[p.Hosts]
		if !ok {
			return fmt.Errorf("prepare %q が指す役割 %q のホストが -role で渡されていません", p.Name, p.Hosts)
		}
		for _, h := range hosts {
			jobs = append(jobs, job{p: p, host: h})
		}
	}
	return parallel(jobs, func(j job) error {
		out, err := r.sshScript(j.host, j.p.Script)
		if err != nil {
			return fmt.Errorf("[%s] %s: %w", j.host, j.p.Name, err)
		}
		r.printLines(j.host, out)
		return nil
	})
}

// oneshotAll は走行中に 1 回だけ実行するものを対象ホストへ並列に流す。
// collector と違って常駐しないので、pid の管理も後片付けも要らない。
func (r *runner) oneshotAll() error {
	type job struct {
		o    Oneshot
		host string
	}
	var jobs []job
	for _, o := range r.cfg.Oneshots {
		if r.only != nil && !r.only[o.Name] {
			continue
		}
		hosts, ok := r.roles[o.Hosts]
		if !ok {
			return fmt.Errorf("oneshot %q が指す役割 %q のホストが -role で渡されていません", o.Name, o.Hosts)
		}
		for _, h := range hosts {
			jobs = append(jobs, job{o: o, host: h})
		}
	}
	return parallel(jobs, func(j job) error {
		if err := r.oneshot(j.o, j.host); err != nil {
			return fmt.Errorf("[%s] %s: %w", j.host, j.o.Name, err)
		}
		return nil
	})
}

func (r *runner) oneshot(o Oneshot, host string) error {
	exp := expander{remoteDir: "", host: host, runID: r.runID, vars: r.vars, remoteOut: o.RemoteOut}
	exp.remoteOut = exp.expand(o.RemoteOut)

	if d := exp.expand(o.Delay); d != "" {
		delay, err := time.ParseDuration(d)
		if err != nil {
			return fmt.Errorf("delay を解釈できません: %w", err)
		}
		if r.dryRun {
			fmt.Printf("[dry-run] sleep %s\n", delay)
		} else {
			time.Sleep(delay)
		}
	}

	run := exp.expand(o.Run)
	if missing := unresolved(run); len(missing) > 0 {
		return fmt.Errorf("コマンドに未解決のプレースホルダがあります: %s", strings.Join(missing, " "))
	}
	if err := r.ssh(host, run); err != nil {
		return err
	}

	local := filepath.Join(r.runDir, exp.expand(o.Output))
	if err := r.rsyncFrom(host, exp.remoteOut, local); err != nil {
		return err
	}
	if !r.dryRun {
		fmt.Printf("[%s] %s 取得\n", host, local)
	}
	return nil
}

// parallel は同じ形の仕事を並列に流し、失敗しても他を止めずに集めて返す。
func parallel[T any](jobs []T, fn func(T) error) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, j := range jobs {
		wg.Add(1)
		go func(j T) {
			defer wg.Done()
			if err := fn(j); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (r *runner) printLines(host, out string) {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			fmt.Printf("[%s] %s\n", host, line)
		}
	}
}

func filterCollectors(all []Collector, want map[string]bool) ([]Collector, error) {
	var out []Collector
	known := make(map[string]bool, len(all))
	for _, c := range all {
		known[c.Name] = true
		if want[c.Name] {
			out = append(out, c)
		}
	}
	if err := validateOnlyNames("collector", want, known); err != nil {
		return nil, err
	}
	return out, nil
}

func validateOnlyNames(kind string, want, known map[string]bool) error {
	for name := range want {
		if !known[name] {
			return fmt.Errorf("-only に一致する %s %q がありません", kind, name)
		}
	}
	return nil
}

func prepareNames(all []Prepare) map[string]bool {
	known := make(map[string]bool, len(all))
	for _, prepare := range all {
		known[prepare.Name] = true
	}
	return known
}

func oneshotNames(all []Oneshot) map[string]bool {
	known := make(map[string]bool, len(all))
	for _, oneshot := range all {
		known[oneshot.Name] = true
	}
	return known
}

// each は collector × ホストを並列に処理し、失敗しても他を止めない。
// 途中で 1 台こけても残りの回収は続けたい (以前は ignore_error をタスクごとに
// 手で貼っていた) ので、エラーは集めて最後にまとめて返す。
func (r *runner) each(fn func(target) error) error {
	targets, err := r.targets()
	if err != nil {
		return err
	}
	return parallel(targets, func(t target) error {
		if err := fn(t); err != nil {
			return fmt.Errorf("[%s] %s: %w", t.host, t.c.Label, err)
		}
		return nil
	})
}

func (r *runner) targets() ([]target, error) {
	var targets []target
	for _, c := range r.cfg.Collectors {
		hosts, ok := r.roles[c.Hosts]
		if !ok {
			return nil, fmt.Errorf("collector %q が指す役割 %q のホストが -role で渡されていません", c.Name, c.Hosts)
		}
		for _, h := range hosts {
			targets = append(targets, target{c: c, host: h})
		}
	}
	return targets, nil
}

func (r *runner) remoteDir(c Collector) string {
	return path.Join(c.RemoteRoot, r.runID)
}

func (r *runner) expander(c Collector, host string) expander {
	return expander{remoteDir: r.remoteDir(c), host: host, runID: r.runID, vars: r.vars}
}

// start はバイナリを配って nohup で起動する。既に走っている collector が
// いれば起動しない (前の走行を止め損ねたまま二重に回すと、どちらの出力か
// 分からないうえ 1 秒間隔の /proc 読みが全走行のノイズになる)。
func (r *runner) start(t target) error {
	c, host := t.c, t.host
	dir := r.remoteDir(c)
	exp := r.expander(c, host)

	args := exp.expand(c.Args)
	if missing := unresolved(args); len(missing) > 0 {
		return fmt.Errorf("引数に未解決のプレースホルダがあります: %s", strings.Join(missing, " "))
	}
	stdout := exp.expand(c.Stdout)
	if stdout == "" {
		stdout = "/dev/null"
	}
	binary := path.Join(dir, filepath.Base(c.Binary))

	if err := r.ssh(host, "sudo mkdir -p "+shq(dir)); err != nil {
		return err
	}
	if err := r.rsyncTo(c.Binary, host, binary); err != nil {
		return fmt.Errorf("バイナリを配れません: %w", err)
	}

	launch := fmt.Sprintf("nohup %s %s >%s 2>%s </dev/null & echo $! >%s",
		binary, args, stdout, path.Join(dir, "collector.stderr"), path.Join(dir, "collector.pid"))
	script := fmt.Sprintf(`set -e
dir=%s
if [ -f "$dir/collector.pid" ]; then
  pid=$(sudo cat "$dir/collector.pid" 2>/dev/null || true)
  case "$pid" in
    ''|*[!0-9]*) ;;
    *) if sudo kill -0 "$pid" 2>/dev/null; then
         echo "collector is already running (pid $pid)" >&2
         exit 1
       fi ;;
  esac
fi
sudo -n sh -c %s
sudo cat "$dir/collector.pid"
`, shq(dir), shq(launch))

	out, err := r.sshScript(host, script)
	if err != nil {
		return err
	}
	if r.dryRun {
		return nil
	}
	fmt.Printf("[%s] %s collector %s started\n", host, c.Label, strings.TrimSpace(out))
	return nil
}

// stop は collector を止めて出力を回収し、リモートの作業ディレクトリを畳む。
// 回収物がローカルに揃っていれば何もしない (after-bench を撮り直しても
// 二度手間にならない)。
func (r *runner) stop(t target) error {
	c, host := t.c, t.host
	dir := r.remoteDir(c)
	exp := r.expander(c, host)

	// リモート名 -> ローカルパス。宣言の outputs をこの走行の実パスへ展開する。
	locals := map[string]string{}
	for remoteName, localName := range c.Outputs {
		locals[remoteName] = filepath.Join(r.runDir, exp.expand(localName))
	}
	if allExist(locals) {
		fmt.Printf("[%s] %s already saved\n", host, c.Label)
		return nil
	}

	stopped := r.terminate(host, dir)

	// collector 自身の stderr も同じ rsync に含める。proc collector は本体だけで
	// 3 ファイルあるため、1 ファイルずつ回収するとホストごとに4回接続することに
	// なる。リモート名を --files-from で1回にまとめ、一時ディレクトリから宣言上の
	// ローカル名へ移す。
	stderrLocal := filepath.Join(r.runDir, exp.expand(c.Stderr))
	files := make(map[string]string, len(locals)+1)
	for remoteName, local := range locals {
		files[remoteName] = local
	}
	files["collector.stderr"] = stderrLocal
	saved, failed, fetchErr := r.rsyncFromMany(host, dir, files)

	if !stopped || len(failed) > 0 || fetchErr != nil {
		return fmt.Errorf("回収に失敗しました (停止=%t, 未回収=%v, rsync=%v)。必要なら after-bench をやり直してください", stopped, failed, fetchErr)
	}

	// rmdir まで済ませないと RUN ごとの空ディレクトリが際限なく溜まる。
	cleanup := "sudo rm -rf " + shq(dir) + " 2>/dev/null || true"
	_ = r.ssh(host, cleanup)

	if !r.dryRun {
		fmt.Printf("[%s] %s: %s saved\n", host, c.Label, strings.Join(saved, " "))
	}
	return nil
}

// terminate は pid ファイルを見て TERM を送り、止まったかどうかを返す。
// pid が無い / 数字でない場合は「既に居ない」とみなして true。
func (r *runner) terminate(host, dir string) bool {
	script := fmt.Sprintf(`dir=%s
pid=$(sudo cat "$dir/collector.pid" 2>/dev/null || true)
case "$pid" in
  ''|*[!0-9]*) echo gone; exit 0 ;;
esac
sudo kill -TERM "$pid" >/dev/null 2>&1 || true
for _ in 1 2 3 4 5; do
  sudo kill -0 "$pid" >/dev/null 2>&1 || { echo stopped; exit 0; }
  sleep 0.2
done
echo "running $pid"
`, shq(dir))
	out, err := r.sshScript(host, script)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(strings.TrimSpace(out), "running")
}

func (r *runner) collectorRoots() []string {
	roots := map[string]bool{}
	for _, c := range r.cfg.Collectors {
		roots[c.RemoteRoot] = true
	}
	return sortedStrings(roots)
}

func (r *runner) allHosts() ([]string, error) {
	hosts, ok := r.roles["all"]
	if !ok || len(hosts) == 0 {
		return nil, errors.New("全ホスト操作に必要な role all が空です")
	}
	unique := map[string]bool{}
	for _, host := range hosts {
		if host != "" {
			unique[host] = true
		}
	}
	if len(unique) == 0 {
		return nil, errors.New("全ホスト操作に必要な role all が空です")
	}
	return sortedStrings(unique), nil
}

func quoted(values []string) []string {
	var quoted []string
	for _, value := range values {
		quoted = append(quoted, shq(value))
	}
	return quoted
}

func (r *runner) runOnAllHosts(label, script string) error {
	hosts, err := r.allHosts()
	if err != nil {
		return err
	}
	return parallel(hosts, func(host string) error {
		out, err := r.sshScript(host, script)
		if err != nil {
			return fmt.Errorf("[%s] %s: %w", host, label, err)
		}
		r.printLines(host, out)
		return nil
	})
}

// checkCleanAll is the pre-run hard gate. It fails if any host still has a
// collector process or a work directory from an earlier run.
func (r *runner) checkCleanAll() error {
	return r.runOnAllHosts("collector clean check", checkCleanScript(quoted(r.collectorRoots())))
}

// sweepAll is used by abort-run. It scans every remote root on role all,
// independently of each collector's current host role. To avoid killing a
// reused PID, the command line must still reference a collector root.
func (r *runner) sweepAll() error {
	return r.runOnAllHosts("collector sweep", sweepScript(quoted(r.collectorRoots())))
}

func checkCleanScript(quotedRoots []string) string {
	return fmt.Sprintf(`found=0
for root in %s; do
  for cmdline in /proc/[0-9]*/cmdline; do
    [ -r "$cmdline" ] || continue
    if tr '\0' ' ' < "$cmdline" | grep -qF "$root/"; then
      pid=${cmdline#/proc/}
      pid=${pid%%/*}
      echo "running collector residue: pid=$pid root=$root" >&2
      found=1
    fi
  done
  [ -d "$root" ] || continue
  for d in "$root"/*/; do
    [ -d "$d" ] || continue
    echo "collector work directory residue: $d" >&2
    found=1
  done
done
if [ "$found" -ne 0 ]; then
  echo "collector residue detected; run 'task abort-run' before starting a benchmark" >&2
  exit 1
fi
`, strings.Join(quotedRoots, " "))
}

func sweepScript(quotedRoots []string) string {
	return fmt.Sprintf(`terminate_collector() {
  pid=$1
  sudo kill -0 "$pid" 2>/dev/null || return 1
  pgid=$(ps -o pgid= -p "$pid" 2>/dev/null | tr -d ' ')
  case "$pgid" in
    ''|*[!0-9]*) pgid= ;;
  esac
  shell_pgid=$(ps -o pgid= -p $$ 2>/dev/null | tr -d ' ')
  if [ -n "$pgid" ] && [ "$pgid" != "$shell_pgid" ]; then
    sudo kill -TERM -- "-$pgid" 2>/dev/null || true
  else
    sudo kill -TERM "$pid" 2>/dev/null || true
  fi
  for _ in 1 2 3 4 5; do
    sudo kill -0 "$pid" 2>/dev/null || break
    sleep 0.2
  done
  if sudo kill -0 "$pid" 2>/dev/null; then
    if [ -n "$pgid" ] && [ "$pgid" != "$shell_pgid" ]; then
      sudo kill -KILL -- "-$pgid" 2>/dev/null || true
    else
      sudo kill -KILL "$pid" 2>/dev/null || true
    fi
  fi
}

for root in %s; do
  killed=0
  # A collector can outlive a removed work directory. Find it by the root in
  # its command line before walking the remaining directories.
  for cmdline in /proc/[0-9]*/cmdline; do
    [ -r "$cmdline" ] || continue
    if tr '\0' ' ' < "$cmdline" | grep -qF "$root/"; then
      pid=${cmdline#/proc/}
      pid=${pid%%/*}
      if terminate_collector "$pid"; then
        killed=$((killed + 1))
        echo "killed $pid (cmdline under $root)"
      fi
    fi
  done
  [ -d "$root" ] || {
    [ "$killed" -eq 0 ] || echo "$root の残留プロセスを掃除 (kill ${killed}件)"
    continue
  }
  for d in "$root"/*/; do
    [ -d "$d" ] || continue
    pid=$(sudo cat "$d/collector.pid" 2>/dev/null || true)
    case "$pid" in
      ''|*[!0-9]*) continue ;;
    esac
    [ -r "/proc/$pid/cmdline" ] || continue
    if tr '\0' ' ' < "/proc/$pid/cmdline" | grep -qF "$d"; then
      if terminate_collector "$pid"; then
        killed=$((killed + 1))
        echo "killed $pid ($d)"
      fi
    fi
  done
  freed=$(sudo du -sh "$root" 2>/dev/null | cut -f1)
  sudo rm -rf "$root"
  echo "$root を掃除 (kill ${killed}件, ${freed:-0} 解放)"
done
`, strings.Join(quotedRoots, " "))
}

// ---- 外部コマンド ------------------------------------------------------

func (r *runner) sshBase() []string {
	return append([]string{"-o", "RequestTTY=no", "-o", "LogLevel=ERROR"}, r.sshOpts...)
}

func (r *runner) ssh(host, remoteCmd string) error {
	args := append(r.sshBase(), r.sshUser+"@"+host, remoteCmd)
	if r.dryRun {
		fmt.Printf("[dry-run] ssh %s\n", strings.Join(args, " "))
		return nil
	}
	out, err := exec.Command("ssh", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// sshScript はスクリプトを stdin から流す。リモートで実行する内容を引数に
// 埋め込まないので、シェルのクォートを二重三重にエスケープせずに済む。
func (r *runner) sshScript(host, script string) (string, error) {
	args := append(r.sshBase(), r.sshUser+"@"+host, "sh -s")
	if r.dryRun {
		fmt.Printf("[dry-run] ssh %s <<'EOF'\n%sEOF\n", strings.Join(args, " "), script)
		return "", nil
	}
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ssh: %w", err)
	}
	return string(out), nil
}

func (r *runner) rsyncArgs() []string {
	return []string{"-az", "--rsync-path=sudo rsync", "-e", "ssh " + strings.Join(r.sshBase(), " ")}
}

func (r *runner) rsyncTo(local, host, remote string) error {
	args := append(r.rsyncArgs(), local, r.sshUser+"@"+host+":"+remote)
	return r.rsync(args)
}

func (r *runner) rsyncFrom(host, remote, local string) error {
	// dry-run では回収先を作らない (空の走行ディレクトリが runs/ に残ってしまう)。
	if !r.dryRun {
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			return err
		}
	}
	args := append(r.rsyncArgs(), r.sshUser+"@"+host+":"+remote, local)
	return r.rsync(args)
}

// rsyncFromMany は同じリモートディレクトリにある複数ファイルを1接続で回収する。
// files の値は宣言で決まる個別の保存名なので、一度 stagingDir に元の名前で受け、
// 同一ファイルシステム内で rename する。rsync が一部失敗しても取得済みファイルは
// 保存し、再実行時に不足分を判断できるようにする。
func (r *runner) rsyncFromMany(host, remoteDir string, files map[string]string) ([]string, []string, error) {
	remoteNames := sortedKeys(files)
	if r.dryRun {
		if err := r.rsyncFilesFrom(host, remoteDir, remoteNames, "<staging-dir>"); err != nil {
			return nil, remoteNames, err
		}
		var saved []string
		for _, remoteName := range remoteNames {
			saved = append(saved, files[remoteName])
		}
		return saved, nil, nil
	}

	if err := os.MkdirAll(r.runDir, 0o755); err != nil {
		return nil, remoteNames, err
	}
	stagingDir, err := os.MkdirTemp(r.runDir, ".collector-fetch-")
	if err != nil {
		return nil, remoteNames, err
	}
	defer os.RemoveAll(stagingDir)

	fetchErr := r.rsyncFilesFrom(host, remoteDir, remoteNames, stagingDir)
	var saved, failed []string
	for _, remoteName := range remoteNames {
		staged := filepath.Join(stagingDir, filepath.FromSlash(remoteName))
		local := files[remoteName]
		if _, err := os.Stat(staged); err != nil {
			failed = append(failed, remoteName)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			failed = append(failed, remoteName)
			continue
		}
		if err := os.Rename(staged, local); err != nil {
			failed = append(failed, remoteName)
			continue
		}
		saved = append(saved, local)
	}
	return saved, failed, fetchErr
}

func (r *runner) rsyncFilesFrom(host, remoteDir string, remoteNames []string, localDir string) error {
	args := append(r.rsyncArgs(), "--files-from=-",
		r.sshUser+"@"+host+":"+strings.TrimSuffix(remoteDir, "/")+"/",
		strings.TrimSuffix(localDir, "/")+"/")
	if r.dryRun {
		fmt.Printf("[dry-run] rsync %s (files: %s)\n", strings.Join(args, " "), strings.Join(remoteNames, ","))
		return nil
	}
	cmd := exec.Command("rsync", args...)
	cmd.Stdin = strings.NewReader(strings.Join(remoteNames, "\n") + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *runner) rsync(args []string) error {
	if r.dryRun {
		fmt.Printf("[dry-run] rsync %s\n", strings.Join(args, " "))
		return nil
	}
	out, err := exec.Command("rsync", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---- 小物 --------------------------------------------------------------

// shq はシェルへ値をそのまま渡すためのシングルクォート。
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func allExist(paths map[string]string) bool {
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return len(paths) > 0
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStrings(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
