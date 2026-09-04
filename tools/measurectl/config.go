package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Collector は collectors.yaml の 1 ブロック。3 種の collector は起動・停止の
// 手順が同型で、違うのはここに書ける差分だけ。
type Collector struct {
	Name             string            `yaml:"name"`
	Label            string            `yaml:"label"`
	EnabledByDefault *bool             `yaml:"enabled_by_default"` // Defaults to true; -include or -only selects false entries.
	Hosts            string            `yaml:"hosts"`              // 役割名。実ホストは -role で解決する
	Binary           string            `yaml:"binary"`             // ローカルのビルド済みバイナリ
	RemoteRoot       string            `yaml:"remote_root"`        // リモート作業ディレクトリの親
	Stdout           string            `yaml:"stdout"`             // バイナリの標準出力のリダイレクト先
	Args             string            `yaml:"args"`
	Outputs          map[string]string `yaml:"outputs"` // リモートのファイル名 -> runs 以下の保存名
	Stderr           string            `yaml:"stderr"`  // collector 自身の stderr の保存名
}

func (c Collector) enabledByDefault() bool {
	return c.EnabledByDefault == nil || *c.EnabledByDefault
}

// Prepare は走行前の下ごしらえ。リモートで流すスクリプトは stdin から渡すので、
// シェルのクォートを多重にエスケープする必要がない。
type Prepare struct {
	Name   string `yaml:"name"`
	Label  string `yaml:"label"`
	Hosts  string `yaml:"hosts"`
	Script string `yaml:"script"`
}

// Oneshot は走行中に 1 回だけ実行して回収するもの。collector と違って
// 常駐しないので、pid の管理も後片付けも要らない。
type Oneshot struct {
	Name             string `yaml:"name"`
	Label            string `yaml:"label"`
	EnabledByDefault *bool  `yaml:"enabled_by_default"` // Defaults to true; false makes its artifact optional.
	Hosts            string `yaml:"hosts"`
	Delay            string `yaml:"delay"`      // 実行前に待つ時間
	Run              string `yaml:"run"`        // リモートで実行するコマンド
	RemoteOut        string `yaml:"remote_out"` // リモートでの出力先
	Output           string `yaml:"output"`     // runs/<RUN_ID>/ での保存名
}

func (o Oneshot) enabledByDefault() bool {
	return o.EnabledByDefault == nil || *o.EnabledByDefault
}

func defaultMeasureConfigPath(name string) string {
	repositoryPath := "tools/measurectl/" + name
	if _, err := os.Stat(repositoryPath); err == nil {
		return repositoryPath
	}
	return name
}

type Config struct {
	Prepare    []Prepare   `yaml:"prepare"`
	Collectors []Collector `yaml:"collectors"`
	Oneshots   []Oneshot   `yaml:"oneshots"`
}

func loadConfig(path string) (*Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("collector 定義を読めません: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("collector 定義を解釈できません: %w", err)
	}
	if len(cfg.Collectors) == 0 && len(cfg.Prepare) == 0 && len(cfg.Oneshots) == 0 {
		return nil, fmt.Errorf("%s に prepare / collectors / oneshots のいずれもありません", path)
	}
	for _, p := range cfg.Prepare {
		if p.Name == "" || p.Hosts == "" || p.Script == "" {
			return nil, fmt.Errorf("prepare %q に name / hosts / script のいずれかが足りません", p.Name)
		}
	}
	for _, o := range cfg.Oneshots {
		if o.Name == "" || o.Hosts == "" || o.Run == "" || o.RemoteOut == "" || o.Output == "" {
			return nil, fmt.Errorf("oneshot %q に name / hosts / run / remote_out / output のいずれかが足りません", o.Name)
		}
	}
	for _, c := range cfg.Collectors {
		if c.Name == "" || c.Hosts == "" || c.Binary == "" || c.RemoteRoot == "" {
			return nil, fmt.Errorf("collector %q に name / hosts / binary / remote_root のいずれかが足りません", c.Name)
		}
		if len(c.Outputs) == 0 {
			return nil, fmt.Errorf("collector %q に outputs がありません", c.Name)
		}
		if c.Stderr == "" {
			return nil, fmt.Errorf("collector %q に stderr がありません", c.Name)
		}
	}
	return &cfg, nil
}

// Source は digesters.yaml が宣言する生ログ 1 種。同じログを複数の集計が読むので、
// 集計とは別に宣言して回収を 1 回だけにする。
type Source struct {
	Name     string `yaml:"name"`
	Role     string `yaml:"role"` // 回収元。実ホストは -role で解決する
	Remote   string `yaml:"remote"`
	Local    string `yaml:"local"`
	Compress bool   `yaml:"compress"` // 集計が済んでから zstd で畳むか
}

// Digester は集計 1 件。ベンチ後の集計は「入力を確かめる → 外部ツールを確かめる →
// タイムアウト付きで実行する → 失敗なら理由を残して既定値で埋める」形が共通なので、
// 違うのはここに書ける宣言だけになる。
// Remote はリモートで SQL などを流し、その標準出力を集計とする指定。
// source の代わりに使う (ローカルの生ログを入力に取らない集計のため)。
type Remote struct {
	Role   string `yaml:"role"`
	Script string `yaml:"script"`
}

type Digester struct {
	Name              string   `yaml:"name"`
	Label             string   `yaml:"label"`
	Source            string   `yaml:"source"`
	Remote            *Remote  `yaml:"remote"`
	Stderr            string   `yaml:"stderr"`               // 失敗理由・スキップ理由の保存名
	Stdin             string   `yaml:"stdin"`                // 入力を stdin へ流すときの読み取りコマンド
	Timeout           string   `yaml:"timeout"`              // 空なら無制限
	SkipIfBenchFailed bool     `yaml:"skip_if_bench_failed"` // preflight 失敗時に集計しない
	InstallHint       string   `yaml:"install_hint"`         // ツールが無いときに添える導入方法
	Outputs           []Output `yaml:"outputs"`
}

// Output は 1 集計が出すファイル。同じ入力を独立に舐めるだけなので並列に流せる。
type Output struct {
	File string `yaml:"file"`
	Run  string `yaml:"run"`
	// 出力に列名が付かないとき (SQL の結果など) に前置するヘッダ行。
	Header string `yaml:"header"`
	// 実行に失敗したときに書き込む内容。空なら集計全体を失敗にする。
	OnError string `yaml:"on_error"`
	// スキップしたときに書き込む内容。空ならファイルを作らない。
	OnSkip string `yaml:"on_skip"`
}

type DigestConfig struct {
	Sources   []Source   `yaml:"sources"`
	Digesters []Digester `yaml:"digesters"`
}

func (c *DigestConfig) source(name string) (Source, bool) {
	for _, s := range c.Sources {
		if s.Name == name {
			return s, true
		}
	}
	return Source{}, false
}

func loadDigestConfig(path string) (*DigestConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("集計の宣言を読めません: %w", err)
	}
	var cfg DigestConfig
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("集計の宣言を解釈できません: %w", err)
	}
	if len(cfg.Digesters) == 0 {
		return nil, fmt.Errorf("%s に digesters がありません", path)
	}
	for _, s := range cfg.Sources {
		if s.Name == "" || s.Role == "" || s.Remote == "" || s.Local == "" {
			return nil, fmt.Errorf("source %q に name / role / remote / local のいずれかが足りません", s.Name)
		}
	}
	for _, d := range cfg.Digesters {
		if d.Name == "" {
			return nil, fmt.Errorf("digester に name がありません")
		}
		switch {
		case d.Source != "" && d.Remote != nil:
			return nil, fmt.Errorf("digester %q は source と remote のどちらか一方だけを指定してください", d.Name)
		case d.Source != "":
			if _, ok := cfg.source(d.Source); !ok {
				return nil, fmt.Errorf("digester %q が指す source %q が宣言にありません", d.Name, d.Source)
			}
		case d.Remote != nil:
			if d.Remote.Role == "" || d.Remote.Script == "" {
				return nil, fmt.Errorf("digester %q の remote に role / script のいずれかが足りません", d.Name)
			}
		default:
			return nil, fmt.Errorf("digester %q に source も remote もありません", d.Name)
		}
		if len(d.Outputs) == 0 {
			return nil, fmt.Errorf("digester %q に outputs がありません", d.Name)
		}
		for _, o := range d.Outputs {
			if o.File == "" {
				return nil, fmt.Errorf("digester %q の outputs に file がありません", d.Name)
			}
			// remote はスクリプトの出力をそのまま使うので run を取らない。
			if d.Remote == nil && o.Run == "" {
				return nil, fmt.Errorf("digester %q の outputs に run がありません", d.Name)
			}
		}
	}
	return &cfg, nil
}

// expander は collectors.yaml のプレースホルダを実際の値へ置き換える。
type expander struct {
	remoteDir string
	remoteOut string
	host      string
	runID     string
	vars      map[string]string
}

var placeholderRe = regexp.MustCompile(`\{(remote_dir|remote_out|host|run_id|var:[A-Za-z0-9_]+)\}`)

func (e expander) expand(s string) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		key := strings.Trim(m, "{}")
		switch key {
		case "remote_dir":
			return e.remoteDir
		case "remote_out":
			return e.remoteOut
		case "host":
			return e.host
		case "run_id":
			return e.runID
		}
		return e.vars[strings.TrimPrefix(key, "var:")]
	})
}

// unresolved は展開しきれなかったプレースホルダを返す。-var の渡し忘れを
// 走行前に落とすために使う (起動してから空文字で動かれるのが一番困る)。
func unresolved(s string) []string {
	var missing []string
	for _, m := range placeholderRe.FindAllString(s, -1) {
		missing = append(missing, m)
	}
	sort.Strings(missing)
	return missing
}

// parseKeyValues は -role app=isucon-1,isucon-2 のような繰り返しフラグを畳む。
type keyValues map[string][]string

func (kv keyValues) String() string { return "" }

func (kv keyValues) Set(v string) error {
	name, value, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("KEY=VALUE の形で指定してください: %q", v)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("キーが空です: %q", v)
	}
	var items []string
	for _, p := range strings.Split(value, ",") {
		if p = strings.TrimSpace(p); p != "" {
			items = append(items, p)
		}
	}
	kv[name] = items
	return nil
}

type keyValue map[string]string

func (kv keyValue) String() string { return "" }

func (kv keyValue) Set(v string) error {
	name, value, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("KEY=VALUE の形で指定してください: %q", v)
	}
	if name = strings.TrimSpace(name); name == "" {
		return fmt.Errorf("キーが空です: %q", v)
	}
	kv[name] = value
	return nil
}
