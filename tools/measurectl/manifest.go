package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Manifest は走行 1 回分の記録。runs/<RUN_ID>/run.json として保存する。
//
// これまで「その RUN で何が取れて何が欠けたか」は、0 バイトの .stderr が
// 残っているかどうかで推測するしかなかった。ここに集約することで、
// 解析側はディレクトリを走査せずに走行の状態を判定できる。
// フィールドは omitempty を付けずに必ず出す。DuckDB の read_json は
// 実ファイルから型を推論するので、RUN によってキーが出たり消えたりすると
// 横断クエリのスキーマが揺れる。
type Manifest struct {
	SchemaVersion   int             `json:"schema_version"`
	Phase           string          `json:"phase"`
	RunID           string          `json:"run_id"`
	StartedAt       string          `json:"started_at"`
	WrittenAt       string          `json:"written_at"`
	FinalizedAt     string          `json:"finalized_at"`
	Score           *int64          `json:"score"`
	Passed          *bool           `json:"passed"`
	Roles           Roles           `json:"roles"`
	Source          CodeSource      `json:"source"`
	BacklogSnapshot BacklogSnapshot `json:"backlog_snapshot"`
	Artifacts       []Artifact      `json:"artifacts"`
	RawBytes        int64           `json:"raw_bytes"`
	Preflight       Preflight       `json:"preflight"`
	Comparison      RunComparison   `json:"comparison"`
	LoadWindow      LoadWindow      `json:"load_window"`
}

type LoadWindow struct {
	StartedAt  string `json:"started_at"`
	EndedAt    string `json:"ended_at"`
	DurationMS int64  `json:"duration_ms"`
	Source     string `json:"source"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
}

type Preflight struct {
	CollectorClean bool `json:"collector_clean"`
}

type RunComparison struct {
	RunID        string   `json:"run_id"`
	Status       string   `json:"status"`
	Reasons      []string `json:"reasons"`
	AllowedCards []string `json:"allowed_cards"`
	AllowedRoles []string `json:"allowed_roles"`
	CardDelta    []string `json:"card_delta"`
	RoleDelta    []string `json:"role_delta"`
}

type BacklogSnapshot struct {
	SchemaVersion int                   `json:"schema_version"`
	Status        string                `json:"status"`
	CapturedAt    string                `json:"captured_at"`
	Revision      int                   `json:"revision"`
	Cards         []AppliedSnapshotCard `json:"cards"`
}

type AppliedSnapshotCard struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	Version        int    `json:"version"`
	Title          string `json:"title"`
	Fingerprint    string `json:"fingerprint"`
	DefinitionHash string `json:"definition_hash"`
}

// Roles はその走行時点のホスト役割。構成をまたぐ RUN 比較で必要になる。
type Roles struct {
	App        []string `json:"app"`
	AppTraffic []string `json:"app_traffic"`
	Nginx      []string `json:"nginx"`
	Entry      string   `json:"entry"`
	MySQL      string   `json:"mysql"`
}

// CodeSource はその走行で動いていたアプリのコード。スコア差分の原因を後から
// 追うとき、RUN と commit の対応が分からないと比較にならない。
// (JSON のキーは source。digesters.yaml の Source とは別物)
type CodeSource struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

// Artifact は回収物 1 件。status は ok / empty / failed のいずれか。
// failed のときは対応する .stderr の中身を reason に入れる。
type Artifact struct {
	Name    string          `json:"name"`
	Bytes   int64           `json:"bytes"`
	Status  string          `json:"status"`
	Reason  string          `json:"reason"`
	Quality ArtifactQuality `json:"quality"`
}

type ArtifactQuality struct {
	Expected          bool    `json:"expected"`
	Status            string  `json:"status"`
	Rows              int64   `json:"rows"`
	InWindowSamples   int64   `json:"in_window_samples"`
	ExpectedSamples   int64   `json:"expected_samples"`
	WindowCoveragePct float64 `json:"window_coverage_pct"`
	MaxGapMS          int64   `json:"max_gap_ms"`
	Monotonic         bool    `json:"monotonic"`
	Finite            bool    `json:"finite"`
	Reason            string  `json:"reason"`
}

var scoreRe = regexp.MustCompile(`(?im)(?:スコア|score)\s*[:：]\s*([0-9]+)`)

// ベンチマーカーは失敗時には結果 JSON、成功時には通常ログを出す。
// preflight の成功だけでは完走を意味しないため、最終チェックの成功だけを
// pass=true として扱う。ログ欠損や途中終了は nil のまま残す。
var passFalseRe = regexp.MustCompile(`(?i)"pass"\s*:\s*false|BENCHMARK_FAIL|(?:整合性|最終)チェック(?:が|に)失敗しました`)
var passTrueRe = regexp.MustCompile(`(?i)"pass"\s*:\s*true|BENCHMARK_PASS|最終チェックが成功しました`)

func runManifest(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("manifest requires begin or finalize")
	}
	switch args[0] {
	case "begin":
		return runManifestBegin(args[1:])
	case "finalize":
		return runManifestFinalize(args[1:])
	default:
		return fmt.Errorf("unknown manifest command %q; use begin or finalize", args[0])
	}
}

func runManifestBegin(args []string) error {
	fs := flag.NewFlagSet("manifest begin", flag.ExitOnError)
	dir := fs.String("dir", "", "走行ディレクトリ (runs/<RUN_ID>)")
	snapshotPath := fs.String("applied-snapshot", "", "before-bench APPLIED snapshot JSON")
	app := fs.String("app", "", "APP_HOSTS (カンマ区切り)")
	appTraffic := fs.String("app-traffic", "", "APP_TRAFFIC_HOSTS (カンマ区切り)")
	nginx := fs.String("nginx", "", "NGINX_HOSTS (カンマ区切り)")
	mysql := fs.String("mysql", "", "MYSQL_HOST")
	entry := fs.String("entry", "", "ENTRY_HOST")
	collectorClean := fs.Bool("collector-clean", false, "collector clean gate passed")
	compareRunDir := fs.String("compare-run-dir", "", "compatible control RUN directory")
	compareAllowedCards := fs.String("compare-allow-cards", "", "card IDs allowed to differ from the control RUN")
	compareAllowedRoles := fs.String("compare-allow-roles", "", "role field names allowed to differ from the control RUN")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return fmt.Errorf("-dir は必須です")
	}
	if _, err := os.Stat(*dir); err != nil {
		return fmt.Errorf("走行ディレクトリを読めません: %w", err)
	}
	if *snapshotPath == "" {
		return fmt.Errorf("-applied-snapshot は必須です")
	}
	body, err := os.ReadFile(*snapshotPath)
	if err != nil {
		return fmt.Errorf("APPLIED snapshotを読めません: %w", err)
	}
	var snapshot BacklogSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return fmt.Errorf("APPLIED snapshotが不正です: %w", err)
	}
	if snapshot.Status != "ok" || snapshot.SchemaVersion < 1 {
		return fmt.Errorf("APPLIED snapshotを利用できません: status=%q schema_version=%d", snapshot.Status, snapshot.SchemaVersion)
	}
	if snapshot.Cards == nil {
		snapshot.Cards = []AppliedSnapshotCard{}
	}
	for _, card := range snapshot.Cards {
		if card.ID == "" || card.Status != "APPLIED" || (card.Kind != "CHANGE" && card.Kind != "MEASUREMENT") || card.DefinitionHash == "" {
			return fmt.Errorf("APPLIED snapshotのカードが不正です: id=%q status=%q kind=%q", card.ID, card.Status, card.Kind)
		}
	}

	runID := filepath.Base(strings.TrimSuffix(*dir, string(filepath.Separator)))
	m := Manifest{
		SchemaVersion: 4,
		Phase:         "started",
		RunID:         runID,
		StartedAt:     parseRunIDTime(runID),
		Roles: Roles{
			App:        splitHosts(*app),
			AppTraffic: splitHosts(*appTraffic),
			Nginx:      splitHosts(*nginx),
			Entry:      *entry,
			MySQL:      *mysql,
		},
		Source:          gitSource(),
		BacklogSnapshot: snapshot,
		Artifacts:       []Artifact{},
		Preflight:       Preflight{CollectorClean: *collectorClean},
		Comparison: RunComparison{
			Status:       "none",
			Reasons:      []string{},
			AllowedCards: splitHosts(*compareAllowedCards),
			AllowedRoles: splitHosts(*compareAllowedRoles),
			CardDelta:    []string{},
			RoleDelta:    []string{},
		},
		LoadWindow: LoadWindow{Status: "pending", Source: "bench.log"},
	}
	if *compareRunDir != "" {
		comparison, err := compareManifest(m, *compareRunDir, m.Comparison.AllowedCards, m.Comparison.AllowedRoles)
		if err != nil {
			return err
		}
		m.Comparison = comparison
		if comparison.Status != "compatible" {
			return fmt.Errorf("control RUN comparison is %s: %s", comparison.Status, strings.Join(comparison.Reasons, "; "))
		}
	}
	if err := writeManifestAtomic(filepath.Join(*dir, "run.json"), m); err != nil {
		return err
	}
	fmt.Printf("%s/run.json (phase=started, APPLIED %d 件, backlog revision %d)\n", *dir, len(snapshot.Cards), snapshot.Revision)
	return nil
}

func runManifestFinalize(args []string) error {
	fs := flag.NewFlagSet("manifest finalize", flag.ExitOnError)
	dir := fs.String("dir", "", "走行ディレクトリ (runs/<RUN_ID>)")
	score := fs.String("score", "", "ポータルのスコア。省略時は bench.log から読む")
	scores := fs.String("scores", "", "スコアと構成の履歴を追記する TSV (省略時は追記しない)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return fmt.Errorf("-dir は必須です")
	}
	path := filepath.Join(*dir, "run.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("manifest beginのrun.jsonを読めません: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return fmt.Errorf("run.jsonが不正です: %w", err)
	}
	if m.SchemaVersion < 2 || (m.Phase != "started" && m.Phase != "finalized") {
		return fmt.Errorf("run.jsonはmanifest beginで作成されたものではありません")
	}
	if m.BacklogSnapshot.Status != "ok" {
		return fmt.Errorf("run.jsonに利用可能なAPPLIED snapshotがありません")
	}
	if *score != "" {
		if _, err := strconv.ParseInt(*score, 10, 64); err != nil || strings.HasPrefix(*score, "-") {
			return fmt.Errorf("-score は 0 以上の整数で指定してください: %q", *score)
		}
		benchPath := filepath.Join(*dir, "bench.log")
		if info, err := os.Stat(benchPath); errors.Is(err, os.ErrNotExist) || (err == nil && info.Size() == 0) {
			body := fmt.Sprintf("本番ポータルから実行\nスコア: %s\n", *score)
			if err := os.WriteFile(benchPath, []byte(body), 0o644); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	benchLog, _ := os.ReadFile(filepath.Join(*dir, "bench.log"))
	m.Score = resolveScore(*score, benchLog)
	m.Passed = resolvePassed(benchLog)
	m.LoadWindow = resolveLoadWindow(benchLog)

	artifacts, rawBytes, err := scanArtifacts(*dir)
	if err != nil {
		return err
	}
	if artifacts == nil {
		artifacts = []Artifact{}
	}
	artifacts = assessArtifactQuality(*dir, m.LoadWindow, artifacts)
	m.Artifacts = artifacts
	m.RawBytes = rawBytes
	m.Phase = "finalized"
	m.FinalizedAt = time.Now().Format(time.RFC3339)
	m.WrittenAt = m.FinalizedAt
	if m.Comparison.RunID != "" {
		controlDir := filepath.Join(filepath.Dir(*dir), m.Comparison.RunID)
		comparison, err := compareManifest(m, controlDir, m.Comparison.AllowedCards, m.Comparison.AllowedRoles)
		if err != nil {
			return err
		}
		m.Comparison = comparison
	}
	if err := writeManifestAtomic(path, m); err != nil {
		return err
	}

	ok, failed := 0, 0
	for _, a := range m.Artifacts {
		if a.Status == "ok" {
			ok++
		} else {
			failed++
		}
	}
	fmt.Printf("%s (成果物 %d 件, 要確認 %d 件, score=%s)\n", path, ok, failed, formatScore(m.Score))

	if *scores != "" {
		if err := appendScores(*scores, m); err != nil {
			return err
		}
		fmt.Printf("スコア %s を %s に追記\n", formatScore(m.Score), *scores)
	}
	return nil
}

func writeManifestAtomic(path string, m Manifest) error {
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".run-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// appendScores は manifest の内容を scores.tsv へ 1 行追記する。
// 走行の記録は run.json が正本で、この TSV はそこから導ける履歴ビュー。
// task runs / task q が読む形式なので、列は変えずに保つ。
func appendScores(path string, m Manifest) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		header := "run_id\tscore\tapp\tnginx\tmysql\tapp_traffic\n"
		if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	// スコアが読めなかった走行も 0 として残す (行が欠けるとRUNの連番が飛ぶ)。
	var score int64
	if m.Score != nil {
		score = *m.Score
	}
	_, err = fmt.Fprintf(f, "%s\t%d\t%s\t%s\t%s\t%s\n",
		m.RunID, score,
		strings.Join(m.Roles.App, ","),
		strings.Join(m.Roles.Nginx, ","),
		m.Roles.MySQL,
		strings.Join(m.Roles.AppTraffic, ","))
	return err
}

// scanArtifacts は走行ディレクトリを 1 段だけ走査して回収物を列挙する。
// raw/ はローカル専用の生ログなので、個別には並べずに合計サイズだけ持つ。
func scanArtifacts(dir string) ([]Artifact, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}

	// .stderr は成果物そのものではなく、対応する回収物の失敗理由。
	// 先に集めてから本体の status 判定に使う。
	reasons := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".stderr") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if text := strings.TrimSpace(string(body)); text != "" {
			reasons[strings.TrimSuffix(name, ".stderr")] = text
		}
	}

	var artifacts []Artifact
	var rawBytes int64
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name == "raw" {
				rawBytes = dirSize(filepath.Join(dir, name))
			}
			continue
		}
		if strings.HasSuffix(name, ".stderr") || name == "run.json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		a := Artifact{Name: name, Bytes: info.Size(), Status: "ok"}
		// 回収物の stem は拡張子を落とした名前 (slp.tsv -> slp)。
		if reason, ok := reasons[strings.TrimSuffix(name, filepath.Ext(name))]; ok {
			a.Status = "failed"
			a.Reason = reason
		} else if info.Size() == 0 {
			a.Status = "empty"
		}
		artifacts = append(artifacts, a)
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Name < artifacts[j].Name })
	return artifacts, rawBytes, nil
}

func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // 読めない枝は数えないだけでよい
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// resolveScore は明示指定を優先し、無ければ bench.log の最後のスコア行を読む。
// どちらも無ければ nil (0 とは区別する。0 点の走行と未記録は別物)。
func resolveScore(explicit string, benchLog []byte) *int64 {
	if explicit != "" {
		if n, err := strconv.ParseInt(explicit, 10, 64); err == nil {
			return &n
		}
	}
	matches := scoreRe.FindAllSubmatch(benchLog, -1)
	if len(matches) == 0 {
		return nil
	}
	if n, err := strconv.ParseInt(string(matches[len(matches)-1][1]), 10, 64); err == nil {
		return &n
	}
	return nil
}

func resolvePassed(benchLog []byte) *bool {
	if passFalseRe.Match(benchLog) {
		v := false
		return &v
	}
	if passTrueRe.Match(benchLog) {
		v := true
		return &v
	}
	return nil
}

func gitSource() CodeSource {
	s := CodeSource{}
	root := ""
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
	}
	if root == "" {
		return s
	}
	if out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); err == nil {
		s.Commit = strings.TrimSpace(string(out))
	}
	// Comparison cleanliness covers every tracked deployment input, not reports
	// or saved RUN artifacts that cannot affect production behavior.
	deploymentPaths := []string{"Taskfile.yml", "webapp", "nginx", "mysql", "etc"}
	gitArgs := append([]string{"-C", root, "status", "--porcelain", "--"}, deploymentPaths...)
	if out, err := exec.Command("git", gitArgs...).Output(); err == nil {
		s.Dirty = strings.TrimSpace(string(out)) != ""
	}
	return s
}

func parseRunIDTime(runID string) string {
	t, err := time.ParseInLocation("20060102-150405", runID, time.Local)
	if err != nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func splitHosts(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func compareManifest(target Manifest, compareRunDir string, allowedCards, allowedRoles []string) (RunComparison, error) {
	body, err := os.ReadFile(filepath.Join(compareRunDir, "run.json"))
	if err != nil {
		return RunComparison{}, fmt.Errorf("compare run manifest cannot be read: %w", err)
	}
	var control Manifest
	if err := json.Unmarshal(body, &control); err != nil {
		return RunComparison{}, fmt.Errorf("compare run manifest is invalid: %w", err)
	}
	requestedControlID := filepath.Base(filepath.Clean(compareRunDir))
	if control.RunID == "" || control.RunID != requestedControlID {
		return RunComparison{}, fmt.Errorf("compare RUN ID mismatch: directory=%q manifest=%q", requestedControlID, control.RunID)
	}
	result := RunComparison{
		RunID:        control.RunID,
		Status:       "compatible",
		Reasons:      []string{},
		AllowedCards: append([]string{}, allowedCards...),
		AllowedRoles: append([]string{}, allowedRoles...),
		CardDelta:    []string{},
		RoleDelta:    []string{},
	}
	incompatible, unverified := false, false
	addReason := func(reason string, hard bool) {
		result.Reasons = append(result.Reasons, reason)
		if hard {
			incompatible = true
		} else {
			unverified = true
		}
	}
	if control.Phase != "finalized" || control.Passed == nil || !*control.Passed {
		addReason("control RUN is not finalized and passed", true)
	}
	if control.SchemaVersion < 3 {
		addReason("control RUN predates the collector-clean comparison contract", false)
	} else if !control.Preflight.CollectorClean {
		addReason("control RUN did not pass the collector-clean gate", true)
	}
	if target.SchemaVersion < 3 || !target.Preflight.CollectorClean {
		addReason("target RUN did not pass the collector-clean gate", true)
	}
	if len(control.Artifacts) == 0 {
		addReason("control RUN has no recorded artifacts", true)
	}
	for _, artifact := range control.Artifacts {
		if artifact.Status != "ok" {
			addReason(fmt.Sprintf("control artifact %s is %s", artifact.Name, artifact.Status), true)
		}
	}
	if target.Source.Dirty || control.Source.Dirty {
		addReason("target or control source is dirty", true)
	}
	if target.Source.Commit == "" || control.Source.Commit == "" {
		addReason("target or control source commit is missing", true)
	}
	if target.Phase == "finalized" {
		if len(target.Artifacts) == 0 {
			addReason("target RUN has no recorded artifacts", true)
		}
		controlArtifactNames := map[string]bool{}
		targetArtifactNames := map[string]bool{}
		for _, artifact := range control.Artifacts {
			controlArtifactNames[artifact.Name] = true
		}
		for _, artifact := range target.Artifacts {
			targetArtifactNames[artifact.Name] = true
		}
		for name := range controlArtifactNames {
			if !targetArtifactNames[name] {
				addReason(fmt.Sprintf("target RUN is missing control artifact %s", name), true)
			}
		}
		for name := range targetArtifactNames {
			if !controlArtifactNames[name] {
				addReason(fmt.Sprintf("target RUN has extra artifact %s", name), true)
			}
		}
		for _, artifact := range target.Artifacts {
			if artifact.Status != "ok" {
				addReason(fmt.Sprintf("target artifact %s is %s", artifact.Name, artifact.Status), true)
			}
		}
	}

	allowedRoleSet := map[string]bool{}
	for _, role := range allowedRoles {
		allowedRoleSet[normalizeRoleField(role)] = true
	}
	targetRoles := roleValues(target.Roles)
	controlRoles := roleValues(control.Roles)
	for role, targetValue := range targetRoles {
		controlValue := controlRoles[role]
		if targetValue == controlValue {
			continue
		}
		result.RoleDelta = append(result.RoleDelta, role)
		if !allowedRoleSet[role] {
			addReason(fmt.Sprintf("role %s differs without allowance: target=%s control=%s", role, targetValue, controlValue), true)
		}
	}
	for role := range allowedRoleSet {
		if _, ok := targetRoles[role]; !ok {
			addReason(fmt.Sprintf("unknown allowed role field %s", role), true)
		} else if targetRoles[role] == controlRoles[role] {
			addReason(fmt.Sprintf("allowed role %s has no actual delta", role), true)
		}
	}

	allowedCardSet := map[string]bool{}
	for _, id := range allowedCards {
		allowedCardSet[strings.ToUpper(strings.TrimSpace(id))] = true
	}
	targetCards := snapshotCardHashes(target.BacklogSnapshot.Cards)
	controlCards := snapshotCardHashes(control.BacklogSnapshot.Cards)
	allCardIDs := map[string]bool{}
	for id := range targetCards {
		allCardIDs[id] = true
	}
	for id := range controlCards {
		allCardIDs[id] = true
	}
	for id := range allCardIDs {
		if targetCards[id] == controlCards[id] {
			continue
		}
		result.CardDelta = append(result.CardDelta, id)
		if !allowedCardSet[id] {
			addReason(fmt.Sprintf("APPLIED card %s differs without allowance", id), true)
		}
	}
	for id := range allowedCardSet {
		if !allCardIDs[id] {
			addReason(fmt.Sprintf("allowed card %s is absent from both snapshots", id), true)
		} else if targetCards[id] == controlCards[id] {
			addReason(fmt.Sprintf("allowed card %s has no actual delta", id), true)
		}
	}
	if target.Source.Commit != "" && control.Source.Commit != "" && target.Source.Commit != control.Source.Commit {
		if len(result.CardDelta) == 0 {
			addReason(fmt.Sprintf("source commit differs without an APPLIED card delta: target=%s control=%s", target.Source.Commit, control.Source.Commit), true)
		} else {
			result.Reasons = append(result.Reasons, fmt.Sprintf("source commit difference is attributed to declared APPLIED card delta: target=%s control=%s", target.Source.Commit, control.Source.Commit))
		}
	}
	sort.Strings(result.Reasons)
	sort.Strings(result.CardDelta)
	sort.Strings(result.RoleDelta)
	if incompatible {
		result.Status = "incompatible"
	} else if unverified {
		result.Status = "unverified"
	}
	return result, nil
}

func normalizeRoleField(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
}

func roleValues(roles Roles) map[string]string {
	return map[string]string{
		"app":         strings.Join(roles.App, ","),
		"app_traffic": strings.Join(roles.AppTraffic, ","),
		"nginx":       strings.Join(roles.Nginx, ","),
		"entry":       roles.Entry,
		"mysql":       roles.MySQL,
	}
}

func snapshotCardHashes(cards []AppliedSnapshotCard) map[string]string {
	result := map[string]string{}
	for _, card := range cards {
		result[strings.ToUpper(card.ID)] = card.DefinitionHash
	}
	return result
}

func formatScore(score *int64) string {
	if score == nil {
		return "unknown"
	}
	return strconv.FormatInt(*score, 10)
}
