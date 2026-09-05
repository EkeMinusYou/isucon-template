package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// 成果物の名前は collectors.yaml / digesters.yaml の宣言が持つ。書き込み側は
// そこに集約されたが、読み手 (dashboard の Go、analysis の SQL) はそれぞれ
// ファイル名を直書きしている。宣言を変えたのに読み手が追随していない、という
// ズレは静かに壊れる:
//
//   - task alp の glob が .log のままで、圧縮後の再集計が空で上書きされた
//   - dashboard の timeline が access-*.log を走行ディレクトリ直下で探していて、
//     raw/ へ移したあと全走行で「データなし」になった
//
// どちらも「宣言が出す名前」と「読み手が読む名前」を突き合わせれば気付けた。
// ここでは宣言を正本として一覧を作り、実走行と読み手の両方に対して照合する。

// ArtifactSpec は「1 走行が出すはずのファイル」1 件。
type ArtifactSpec struct {
	// 走行ディレクトリからの相対パス。ホスト差し替えは * にしてある。
	Pattern string
	// 誰が出すか (collector:proc / digester:alp など)。ズレたときに追う先。
	Producer string
	// 正常時に残らないもの (stderr は空なら after-bench が消す)。
	Optional bool
}

func runArtifacts(args []string) error {
	fs := flag.NewFlagSet("artifacts", flag.ExitOnError)
	collectors := fs.String("collectors", defaultMeasureConfigPath("collectors.yaml"), "collector の宣言")
	digesters := fs.String("digesters", defaultMeasureConfigPath("digesters.yaml"), "集計の宣言")
	runDir := fs.String("run-dir", "", "実走行を照合する (runs/<RUN_ID>)")
	check := fs.Bool("check", false, "読み手 (analysis の SQL / dashboard の Go) と照合する")
	if err := fs.Parse(args); err != nil {
		return err
	}

	specs, err := loadArtifactSpecs(*collectors, *digesters)
	if err != nil {
		return err
	}

	switch {
	case *runDir != "":
		return checkRunDir(*runDir, specs)
	case *check:
		return checkReaders(specs)
	default:
		for _, s := range specs {
			opt := ""
			if s.Optional {
				opt = "  (optional)"
			}
			fmt.Printf("%-34s %s%s\n", s.Pattern, s.Producer, opt)
		}
		return nil
	}
}

// loadArtifactSpecs は 2 つの宣言から、1 走行が出すファイルの一覧を作る。
func loadArtifactSpecs(collectorsPath, digestersPath string) ([]ArtifactSpec, error) {
	var specs []ArtifactSpec

	cfg, err := loadConfig(collectorsPath)
	if err != nil {
		return nil, err
	}
	for _, c := range cfg.Collectors {
		for _, local := range c.Outputs {
			specs = append(specs, ArtifactSpec{
				Pattern:  hostGlob(local),
				Producer: "collector:" + c.Name,
				Optional: !c.enabledByDefault(),
			})
		}
		specs = append(specs, ArtifactSpec{
			Pattern:  hostGlob(c.Stderr),
			Producer: "collector:" + c.Name,
			Optional: true,
		})
	}
	for _, o := range cfg.Oneshots {
		specs = append(specs, ArtifactSpec{
			Pattern:  hostGlob(o.Output),
			Producer: "oneshot:" + o.Name,
			Optional: !o.enabledByDefault(),
		})
	}

	dcfg, err := loadDigestConfig(digestersPath)
	if err != nil {
		return nil, err
	}
	for _, s := range dcfg.Sources {
		// 走行ディレクトリの外に置く生ログ (raw/mysql-slow.log) は
		// 走行ごとの成果物ではないので数えない。
		local := hostGlob(s.Local)
		if !strings.Contains(local, "{run_dir}") {
			continue
		}
		local = strings.TrimPrefix(strings.ReplaceAll(local, "{run_dir}", ""), "/")
		if s.Compress {
			// 集計が済むと圧縮されるので、残る実体は .zst。ただし回収直後は
			// 非圧縮なので、読み手が両方を見るのは正しい。両方を宣言する。
			specs = append(specs,
				ArtifactSpec{Pattern: local + ".zst", Producer: "source:" + s.Name},
				ArtifactSpec{Pattern: local, Producer: "source:" + s.Name + " (圧縮前)", Optional: true},
			)
			continue
		}
		specs = append(specs, ArtifactSpec{Pattern: local, Producer: "source:" + s.Name})
	}
	for _, d := range dcfg.Digesters {
		for _, o := range d.Outputs {
			specs = append(specs, ArtifactSpec{
				Pattern:  o.File,
				Producer: "digester:" + d.Name,
				Optional: !d.enabledByDefault(),
			})
		}
		specs = append(specs, ArtifactSpec{
			Pattern:  d.Stderr,
			Producer: "digester:" + d.Name,
			Optional: true,
		})
	}

	// 宣言ではなく measurectl 自身やベンチが書くもの。
	specs = append(specs,
		ArtifactSpec{Pattern: "run.json", Producer: "measurectl:manifest"},
		ArtifactSpec{Pattern: "bench.log", Producer: "bench"},
		// 走行ディレクトリではなく runs/ 直下に積む履歴。
		ArtifactSpec{Pattern: "scores.tsv", Producer: "measurectl:manifest", Optional: true},
		ArtifactSpec{Pattern: "outcomes.tsv", Producer: "task pass", Optional: true},
	)

	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Pattern != specs[j].Pattern {
			return specs[i].Pattern < specs[j].Pattern
		}
		return specs[i].Producer < specs[j].Producer
	})
	return specs, nil
}

// checkRunDir は実走行に、宣言が出すはずのファイルが揃っているかを見る。
func checkRunDir(dir string, specs []ArtifactSpec) error {
	var missing []string
	for _, s := range specs {
		if s.Optional {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(dir, s.Pattern))
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			missing = append(missing, fmt.Sprintf("  %-34s %s", s.Pattern, s.Producer))
		}
	}
	if len(missing) == 0 {
		fmt.Printf("%s: 宣言どおりの成果物が揃っています\n", dir)
		return nil
	}
	fmt.Printf("%s に無い成果物:\n%s\n", dir, strings.Join(missing, "\n"))
	fmt.Println("\nベンチが途中で落ちた走行では欠けるのが正常です。run.json の artifacts で理由を確認してください。")
	return errors.New("必須成果物が欠けています")
}

// appendMissingArtifacts records required outputs that were never created.
// run.json itself is excluded because the manifest must not list itself.
func appendMissingArtifacts(dir string, artifacts []Artifact, specs []ArtifactSpec) ([]Artifact, error) {
	for _, spec := range specs {
		if spec.Optional || spec.Pattern == "run.json" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(dir, spec.Pattern))
		if err != nil {
			return nil, fmt.Errorf("invalid artifact pattern %q: %w", spec.Pattern, err)
		}
		if len(matches) == 0 {
			artifacts = append(artifacts, Artifact{
				Name:   spec.Pattern,
				Status: "missing",
				Reason: fmt.Sprintf("%s did not create a matching artifact", spec.Producer),
			})
		}
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Name < artifacts[j].Name })
	return artifacts, nil
}

// readerSources は成果物を読む側。ここを増やしたら追記する。
var readerSources = []struct {
	name string
	glob string
}{
	{"analysis(SQL)", "tools/analysis/schema/*.sql"},
	{"dashboard(Go)", "tools/dashboard/server/*.go"},
}

// 読み手が成果物を指すのは必ず文字列リテラルの中なので、そこだけを見る。
// これを外すと SQL のカラム参照 (r.json) まで拾ってしまう。
var (
	goStringRe  = regexp.MustCompile(`"([^"\\\n]|\\.)*"`)
	sqlStringRe = regexp.MustCompile(`'[^'\n]*'`)
)

// 成果物として扱う拡張子。宣言に無い圧縮形式へ読み手だけが移ったときに
// 気付けるよう、いま使っていない形式も含める。
// dashboard が動的に生成する応答 (fgprof の graph.svg) は runs に置かれる
// ファイルではないので入れない。
var artifactExts = map[string]bool{
	".tsv": true, ".csv": true, ".json": true, ".log": true, ".txt": true,
	".pprof": true, ".stderr": true,
	".zst": true, ".gz": true, ".bz2": true, ".xz": true,
}

func checkReaders(specs []ArtifactSpec) error {
	var problems []string
	for _, r := range readerSources {
		files, err := filepath.Glob(r.glob)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			problems = append(problems, fmt.Sprintf("  %s: ソースが見つかりません (%s)", r.name, r.glob))
			continue
		}

		used := map[string]bool{}
		for _, f := range files {
			// テストは固定のホスト名や作り物のファイルを書くので照合しない。
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			body, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			for _, name := range artifactNamesIn(string(body), filepath.Ext(f)) {
				used[name] = true
			}
		}

		for _, name := range sortedSet(used) {
			if matchesAnySpec(name, specs) {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"  %s が %q を読んでいますが、宣言にありません", r.name, name))
		}
	}

	if len(problems) > 0 {
		fmt.Println("宣言と読み手がずれています:")
		fmt.Println(strings.Join(problems, "\n"))
		fmt.Println("\n宣言 (tools/measurectl/*.yaml) を変えたら、読み手も追随させてください。")
		return errors.New("宣言と読み手の不一致")
	}
	fmt.Println("宣言と読み手は一致しています")
	return nil
}

// artifactNamesIn は 1 ファイルの文字列リテラルから成果物名を拾う。
//
// リテラルの中を部分一致で探すのではなく、リテラル全体を 1 つの名前として
// 扱う。部分一致にすると "access-*.log.gz" から "access-*.log" だけを
// 切り出してしまい、宣言に無い圧縮形式へ移ったことを見逃す。
func artifactNamesIn(body, ext string) []string {
	re := goStringRe
	if ext == ".sql" {
		re = sqlStringRe
	}
	var out []string
	for _, lit := range re.FindAllString(body, -1) {
		name := path.Base(strings.Trim(lit, `"'`))
		if !artifactExts[path.Ext(name)] {
			continue
		}
		if isArtifactName(name) {
			out = append(out, name)
		}
	}
	return out
}

// matchesAnySpec は、読み手が使っている名前が宣言のどれかに当たるかを見る。
// 宣言側は {host} を * にしてあり、読み手側は "-proc-metrics.tsv" のように
// 接頭辞を削った断片を書くことがあるので、glob として双方向に照合する。
func matchesAnySpec(name string, specs []ArtifactSpec) bool {
	for _, s := range specs {
		base := filepath.Base(s.Pattern)
		if base == name {
			return true
		}
		if ok, _ := filepath.Match(base, name); ok {
			return true
		}
		if ok, _ := filepath.Match(name, base); ok {
			return true
		}
	}
	return false
}

// isArtifactName は、拾った文字列が走行の成果物らしいかを判定する。
// ソースには go.sum や README.md のような無関係な名前も現れる。
func isArtifactName(s string) bool {
	switch {
	case strings.HasSuffix(s, ".md"), strings.HasSuffix(s, ".sum"), strings.HasSuffix(s, ".mod"):
		return false
	case strings.HasPrefix(s, "."): // .gitignore など
		return false
	case !strings.ContainsAny(s, "-*."):
		return false
	}
	return true
}

// hostGlob は宣言の {host} を、実ファイルを探すための * に均す。
func hostGlob(s string) string {
	return strings.ReplaceAll(s, "{host}", "*")
}

func suffixIf(cond bool, suffix string) string {
	if cond {
		return suffix
	}
	return ""
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
