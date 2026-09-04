package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type verificationContract struct {
	Version   int                    `json:"version"`
	Artifacts []string               `json:"artifacts"`
	Endpoints []verificationEndpoint `json:"endpoints"`
	Profiles  []verificationProfile  `json:"profiles"`
	TSV       []verificationTSV      `json:"tsv"`
	Checks    []string               `json:"checks"`
	Note      string                 `json:"note"`
}

type verificationEndpoint struct {
	Name     string   `json:"name"`
	Artifact string   `json:"artifact"`
	Method   string   `json:"method"`
	URI      string   `json:"uri"`
	Metrics  []string `json:"metrics"`
}

type verificationProfile struct {
	Name     string   `json:"name"`
	Artifact string   `json:"artifact"`
	Binary   string   `json:"binary"`
	Symbol   string   `json:"symbol"`
	Absent   []string `json:"absent"`
}

type verificationTSV struct {
	Name     string            `json:"name"`
	Artifact string            `json:"artifact"`
	Match    map[string]string `json:"match"`
	Metrics  []string          `json:"metrics"`
}

type verifyManifest struct {
	SchemaVersion   int              `json:"schema_version"`
	RunID           string           `json:"run_id"`
	Phase           string           `json:"phase"`
	StartedAt       string           `json:"started_at"`
	Score           *int64           `json:"score"`
	Passed          *bool            `json:"passed"`
	Roles           verifyRoles      `json:"roles"`
	Source          verifyCodeSource `json:"source"`
	Artifacts       []verifyArtifact `json:"artifacts"`
	Comparison      runComparison    `json:"comparison"`
	BacklogSnapshot AppliedSnapshot  `json:"backlog_snapshot"`
	Dir             string           `json:"-"`
}

type verifyRoles struct {
	App        []string `json:"app"`
	AppTraffic []string `json:"app_traffic"`
	Nginx      []string `json:"nginx"`
	MySQL      string   `json:"mysql"`
}

type verifyCodeSource struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

type verifyArtifact struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type verificationSummary struct {
	CardID     string               `json:"card_id"`
	Title      string               `json:"title"`
	Status     string               `json:"status"`
	TargetRun  runSummary           `json:"target_run"`
	CompareRun *runSummary          `json:"compare_run,omitempty"`
	Contract   string               `json:"contract"`
	Artifacts  []artifactSummary    `json:"artifacts,omitempty"`
	Endpoints  []metricItemSummary  `json:"endpoints,omitempty"`
	Profiles   []profileItemSummary `json:"profiles,omitempty"`
	TSV        []metricItemSummary  `json:"tsv,omitempty"`
	Checks     []string             `json:"checks,omitempty"`
	Note       string               `json:"note,omitempty"`
	Warnings   []string             `json:"warnings,omitempty"`
}

type runSummary struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Score  *int64 `json:"score"`
	Passed *bool  `json:"passed"`
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

type artifactSummary struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type metricItemSummary struct {
	Name       string            `json:"name"`
	Artifact   string            `json:"artifact"`
	Lookup     string            `json:"lookup"`
	Current    map[string]string `json:"current,omitempty"`
	Comparison map[string]string `json:"comparison,omitempty"`
	Warning    string            `json:"warning,omitempty"`
}

type profileItemSummary struct {
	Name       string          `json:"name"`
	Artifact   string          `json:"artifact"`
	Symbol     string          `json:"symbol"`
	Current    *profileSummary `json:"current,omitempty"`
	Comparison *profileSummary `json:"comparison,omitempty"`
	Warning    string          `json:"warning,omitempty"`
}

type profileSummary struct {
	SymbolFound bool            `json:"symbol_found"`
	Absent      map[string]bool `json:"absent"`
}

func runEvidence(config cliConfig, args []string) {
	format, runID, idsArg, err := parseVerifyArgs(args)
	if err != nil {
		fatal(err)
	}
	ids, err := parseCardIDList(idsArg)
	if err != nil {
		fatal(err)
	}
	manifests, err := loadVerifyManifests(filepath.Join(config.root, "runs"))
	if err != nil {
		fatal(err)
	}
	if len(manifests) == 0 {
		fatal(errors.New("runs/*/run.json がありません"))
	}

	store := openCLIStore(config)
	defer store.Close()
	summaries := make([]verificationSummary, 0, len(ids))
	for _, id := range ids {
		card, err := store.getCard(id)
		if err != nil {
			fatal(err)
		}
		target, err := selectEvidenceManifest(card.ID, runID, manifests)
		if err != nil {
			fatal(err)
		}
		summary, err := buildVerificationSummary(config.root, card, target, manifests)
		if err != nil {
			fatal(fmt.Errorf("%s: %w", id, err))
		}
		summaries = append(summaries, summary)
	}
	if format == "json" {
		out, err := json.MarshalIndent(summaries, "", "  ")
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(out))
		return
	}
	printVerificationSummaries(summaries)
}

func parseVerifyArgs(args []string) (string, string, string, error) {
	format := "text"
	runID := ""
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-format", "--format":
			if i+1 >= len(args) {
				return "", "", "", errors.New("-format requires text or json")
			}
			i++
			format = strings.ToLower(args[i])
		case "-run", "--run":
			if i+1 >= len(args) {
				return "", "", "", errors.New("--run requires a RUN ID")
			}
			i++
			runID = strings.TrimSpace(args[i])
		default:
			positional = append(positional, args[i])
		}
	}
	if format != "text" && format != "json" {
		return "", "", "", fmt.Errorf("unknown format %q", format)
	}
	if len(positional) != 1 {
		return "", "", "", errors.New("evidence requires one comma-separated card ID argument")
	}
	return format, runID, positional[0], nil
}

func loadVerifyManifests(runsDir string) ([]verifyManifest, error) {
	paths, err := filepath.Glob(filepath.Join(runsDir, "*", "run.json"))
	if err != nil {
		return nil, err
	}
	var manifests []verifyManifest
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var manifest verifyManifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if manifest.SchemaVersion != 4 {
			continue
		}
		if manifest.Phase != "finalized" {
			continue
		}
		directoryRunID := filepath.Base(filepath.Dir(path))
		if manifest.RunID == "" || manifest.RunID != directoryRunID {
			return nil, fmt.Errorf("read %s: run_id %q does not match directory %q", path, manifest.RunID, directoryRunID)
		}
		manifest.Dir = filepath.Dir(path)
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].RunID < manifests[j].RunID })
	return manifests, nil
}

func buildVerificationSummary(root string, card Card, target verifyManifest, manifests []verifyManifest) (verificationSummary, error) {
	compare, comparisonWarning := selectDeclaredCompareManifest(target, manifests)
	summary := verificationSummary{
		CardID: card.ID, Title: card.Title, Status: card.Status,
		TargetRun: summarizeRun(root, target), Contract: "missing",
	}
	if comparisonWarning != "" {
		summary.Warnings = append(summary.Warnings, comparisonWarning)
	} else if compare != nil {
		r := summarizeRun(root, *compare)
		summary.CompareRun = &r
	}
	if card.Status != "APPLIED" && card.Status != "VALIDATED" {
		summary.Warnings = append(summary.Warnings, "カード状態はAPPLIED/VALIDATEDではありません: "+card.Status)
	}
	if snapshotCard, ok := appliedSnapshotCard(target.BacklogSnapshot, card.ID); ok {
		if snapshotCard.ChangeBoundaryHash != cardChangeBoundaryHash(card) {
			summary.Warnings = append(summary.Warnings, "現在のChange boundaryは対象RUNのsnapshotから変更されています")
		}
		if snapshotCard.DecisionHash != cardDecisionHash(card) {
			summary.Warnings = append(summary.Warnings, "現在の仮説・検証・安全条件は対象RUNのsnapshotから変更されています")
		}
	}
	if target.Passed == nil {
		summary.Warnings = append(summary.Warnings, "対象RUNのpass判定がありません")
	} else if !*target.Passed {
		summary.Warnings = append(summary.Warnings, "対象RUNはpass=falseです")
	}

	body := sectionBody(card.Sections, sectionVerification)
	if strings.TrimSpace(body) == "" {
		return summary, nil
	}
	contract, err := parseVerificationContract(body)
	if err != nil {
		trimmed := strings.TrimSpace(body)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "```") {
			return summary, err
		}
		summary.Contract = "free text"
		summary.Checks = []string{trimmed}
		return summary, nil
	}
	summary.Contract = "version 1"
	summary.Checks = append(summary.Checks, contract.Checks...)
	summary.Note = contract.Note
	for _, name := range contract.Artifacts {
		summary.Artifacts = append(summary.Artifacts, summarizeArtifact(root, target, name))
	}
	for _, item := range contract.Endpoints {
		summary.Endpoints = append(summary.Endpoints, summarizeEndpoint(root, target, compare, item))
	}
	for _, item := range contract.Profiles {
		summary.Profiles = append(summary.Profiles, summarizeProfile(root, target, compare, item))
	}
	for _, item := range contract.TSV {
		summary.TSV = append(summary.TSV, summarizeTSV(root, target, compare, item))
	}
	return summary, nil
}

func parseVerificationContract(body string) (verificationContract, error) {
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "```") {
		firstNewline := strings.IndexByte(body, '\n')
		lastFence := strings.LastIndex(body, "```")
		if firstNewline < 0 || lastFence <= firstNewline {
			return verificationContract{}, errors.New("Verification code fence is incomplete")
		}
		body = strings.TrimSpace(body[firstNewline+1 : lastFence])
	}
	var contract verificationContract
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return contract, fmt.Errorf("invalid Verification JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return contract, errors.New("invalid Verification JSON: trailing content")
	}
	if contract.Version != 1 {
		return contract, fmt.Errorf("unsupported Verification version %d", contract.Version)
	}
	return contract, nil
}

func appliedSnapshotCard(snapshot AppliedSnapshot, cardID string) (AppliedSnapshotCard, bool) {
	if snapshot.Status != "ok" || snapshot.SchemaVersion != 3 {
		return AppliedSnapshotCard{}, false
	}
	for _, card := range snapshot.Cards {
		if card.ID == cardID && card.Status == "APPLIED" {
			return card, true
		}
	}
	return AppliedSnapshotCard{}, false
}

func selectEvidenceManifest(cardID, requestedRunID string, manifests []verifyManifest) (verifyManifest, error) {
	if requestedRunID != "" {
		for _, manifest := range manifests {
			if manifest.RunID != requestedRunID {
				continue
			}
			if _, ok := appliedSnapshotCard(manifest.BacklogSnapshot, cardID); !ok {
				return verifyManifest{}, fmt.Errorf("card %s was not APPLIED in RUN %s snapshot", cardID, requestedRunID)
			}
			return manifest, nil
		}
		return verifyManifest{}, fmt.Errorf("finalized RUN %s not found", requestedRunID)
	}
	for i := len(manifests) - 1; i >= 0; i-- {
		if _, ok := appliedSnapshotCard(manifests[i].BacklogSnapshot, cardID); ok {
			return manifests[i], nil
		}
	}
	return verifyManifest{}, fmt.Errorf("no finalized RUN snapshot contains APPLIED card %s", cardID)
}

func selectDeclaredCompareManifest(target verifyManifest, manifests []verifyManifest) (*verifyManifest, string) {
	if target.Comparison.RunID == "" || target.Comparison.Status == "none" {
		return nil, "対象RUNに比較RUNは宣言されていません"
	}
	if target.Comparison.Status != "compatible" {
		return nil, fmt.Sprintf("対象RUNの比較 %s は %s です", target.Comparison.RunID, target.Comparison.Status)
	}
	if target.Comparison.RunID == target.RunID {
		return nil, "対象RUN自身を比較RUNとして使用できません"
	}
	for i := range manifests {
		if manifests[i].RunID == target.Comparison.RunID {
			return &manifests[i], ""
		}
	}
	return nil, fmt.Sprintf("宣言された比較RUN %s のfinalized manifestがありません", target.Comparison.RunID)
}

func summarizeRun(root string, manifest verifyManifest) runSummary {
	return runSummary{ID: manifest.RunID, Path: relativePath(root, manifest.Dir), Score: manifest.Score, Passed: manifest.Passed, Commit: manifest.Source.Commit, Dirty: manifest.Source.Dirty}
}

func summarizeArtifact(root string, manifest verifyManifest, name string) artifactSummary {
	result := artifactSummary{Name: name, Path: relativePath(root, filepath.Join(manifest.Dir, name)), Status: "missing"}
	for _, artifact := range manifest.Artifacts {
		if artifact.Name == name {
			result.Status, result.Reason = artifact.Status, artifact.Reason
			return result
		}
	}
	if info, err := os.Stat(filepath.Join(manifest.Dir, name)); err == nil && info.Size() > 0 {
		result.Status = "ok (unlisted)"
	}
	return result
}

func summarizeEndpoint(root string, target verifyManifest, compare *verifyManifest, item verificationEndpoint) metricItemSummary {
	result := metricItemSummary{Name: item.Name, Artifact: relativePath(root, filepath.Join(target.Dir, item.Artifact)), Lookup: item.Method + " " + item.URI}
	current, err := readALPRow(filepath.Join(target.Dir, item.Artifact), item.Method, item.URI, item.Metrics)
	if err != nil {
		result.Warning = err.Error()
		return result
	}
	result.Current = current
	if compare != nil {
		values, err := readALPRow(filepath.Join(compare.Dir, item.Artifact), item.Method, item.URI, item.Metrics)
		if err != nil {
			result.Warning = "compare: " + err.Error()
		} else {
			result.Comparison = values
		}
	}
	return result
}

func readALPRow(path, method, uri string, metrics []string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows [][]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&rows); err != nil || len(rows) < 1 {
		return nil, fmt.Errorf("invalid alp JSON: %w", err)
	}
	header := map[string]int{}
	for i, value := range rows[0] {
		header[fmt.Sprint(value)] = i
	}
	for _, row := range rows[1:] {
		if valueAt(row, header["method"]) != method || valueAt(row, header["uri"]) != uri {
			continue
		}
		result := map[string]string{}
		for _, metric := range metrics {
			index, ok := header[metric]
			if !ok {
				return nil, fmt.Errorf("alp column %q not found", metric)
			}
			result[metric] = valueAt(row, index)
		}
		return result, nil
	}
	return nil, fmt.Errorf("alp row not found: %s %s", method, uri)
}

func summarizeTSV(root string, target verifyManifest, compare *verifyManifest, item verificationTSV) metricItemSummary {
	result := metricItemSummary{Name: item.Name, Artifact: relativePath(root, filepath.Join(target.Dir, item.Artifact)), Lookup: formatMatch(item.Match)}
	values, err := readTSVRow(filepath.Join(target.Dir, item.Artifact), item.Match, item.Metrics)
	if err != nil {
		result.Warning = err.Error()
		return result
	}
	result.Current = values
	if compare != nil {
		values, err := readTSVRow(filepath.Join(compare.Dir, item.Artifact), item.Match, item.Metrics)
		if err != nil {
			result.Warning = "compare: " + err.Error()
		} else {
			result.Comparison = values
		}
	}
	return result
}

func readTSVRow(path string, match map[string]string, metrics []string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(bufio.NewReader(file))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	headerRow, err := reader.Read()
	if err != nil {
		return nil, err
	}
	header := map[string]int{}
	for i, name := range headerRow {
		header[name] = i
	}
	var selected []string
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		matched := true
		for key, expected := range match {
			index, ok := header[key]
			if !ok || valueAtStrings(row, index) != expected {
				matched = false
				break
			}
		}
		if matched {
			selected = row
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("TSV row not found: %s", formatMatch(match))
	}
	result := map[string]string{}
	for _, metric := range metrics {
		index, ok := header[metric]
		if !ok {
			return nil, fmt.Errorf("TSV column %q not found", metric)
		}
		result[metric] = valueAtStrings(selected, index)
	}
	return result, nil
}

func summarizeProfile(root string, target verifyManifest, compare *verifyManifest, item verificationProfile) profileItemSummary {
	result := profileItemSummary{Name: item.Name, Artifact: relativePath(root, filepath.Join(target.Dir, item.Artifact)), Symbol: item.Symbol}
	current, err := readProfile(root, target, item)
	if err != nil {
		result.Warning = err.Error()
		return result
	}
	result.Current = &current
	if compare != nil {
		value, err := readProfile(root, *compare, item)
		if err != nil {
			result.Warning = "compare: " + err.Error()
		} else {
			result.Comparison = &value
		}
	}
	return result
}

func readProfile(root string, manifest verifyManifest, item verificationProfile) (profileSummary, error) {
	binary := item.Binary
	if !filepath.IsAbs(binary) {
		binary = filepath.Join(root, binary)
	}
	profile := filepath.Join(manifest.Dir, item.Artifact)
	output, err := exec.Command("go", "tool", "pprof", "-list="+item.Symbol, binary, profile).CombinedOutput()
	if err != nil {
		return profileSummary{}, fmt.Errorf("pprof: %s", strings.TrimSpace(string(output)))
	}
	text := string(output)
	result := profileSummary{SymbolFound: strings.TrimSpace(text) != "", Absent: map[string]bool{}}
	for _, name := range item.Absent {
		result.Absent[name] = !strings.Contains(text, name)
	}
	return result, nil
}

func printVerificationSummaries(summaries []verificationSummary) {
	for index, summary := range summaries {
		if index > 0 {
			fmt.Println()
		}
		fmt.Printf("%s %s [%s]\n", summary.CardID, summary.Title, summary.Status)
		fmt.Printf("  target:  %s score=%s pass=%s source=%s dirty=%t (%s)\n", summary.TargetRun.ID, pointerInt(summary.TargetRun.Score), pointerBool(summary.TargetRun.Passed), summary.TargetRun.Commit, summary.TargetRun.Dirty, summary.TargetRun.Path)
		if summary.CompareRun != nil {
			fmt.Printf("  compare: %s score=%s pass=%s (%s)\n", summary.CompareRun.ID, pointerInt(summary.CompareRun.Score), pointerBool(summary.CompareRun.Passed), summary.CompareRun.Path)
		}
		fmt.Printf("  contract: %s\n", summary.Contract)
		for _, artifact := range summary.Artifacts {
			fmt.Printf("  artifact %-28s %-12s %s\n", artifact.Name, artifact.Status, artifact.Path)
		}
		printMetricItems("endpoint", summary.Endpoints)
		printMetricItems("tsv", summary.TSV)
		for _, item := range summary.Profiles {
			fmt.Printf("  profile  %s (%s) symbol=%s", item.Name, item.Artifact, item.Symbol)
			if item.Warning != "" {
				fmt.Printf(" warning=%s", item.Warning)
			} else {
				fmt.Printf(" current=%s compare=%s", profileText(item.Current), profileText(item.Comparison))
			}
			fmt.Println()
		}
		for _, check := range summary.Checks {
			fmt.Printf("  check: %s\n", check)
		}
		if summary.Note != "" {
			fmt.Printf("  note: %s\n", summary.Note)
		}
		for _, warning := range summary.Warnings {
			fmt.Printf("  warning: %s\n", warning)
		}
	}
}

func printMetricItems(kind string, items []metricItemSummary) {
	for _, item := range items {
		fmt.Printf("  %-8s %s (%s) lookup=%s", kind, item.Name, item.Artifact, item.Lookup)
		if item.Warning != "" {
			fmt.Printf(" warning=%s", item.Warning)
		} else {
			fmt.Printf(" current=%s compare=%s", formatValues(item.Current), formatValues(item.Comparison))
		}
		fmt.Println()
	}
}

func valueAt(row []any, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return fmt.Sprint(row[index])
}

func valueAtStrings(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

func formatMatch(match map[string]string) string {
	keys := make([]string, 0, len(match))
	for key := range match {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+match[key])
	}
	return strings.Join(parts, ",")
}

func formatValues(values map[string]string) string {
	if len(values) == 0 {
		return "-"
	}
	return formatMatch(values)
}

func profileText(value *profileSummary) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("symbol=%t absent={%s}", value.SymbolFound, formatBoolMap(value.Absent))
}

func formatBoolMap(values map[string]bool) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%t", key, values[key]))
	}
	return strings.Join(parts, ",")
}

func pointerInt(value *int64) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprint(*value)
}

func pointerBool(value *bool) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprint(*value)
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return relative
}
