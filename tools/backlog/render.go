package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
)

var useColor bool

var statusOrder = []string{"INVESTIGATE", "READY", "DOING", "VERIFY", "APPLIED", "BLOCKED", "VALIDATED", "REJECTED"}
var runPattern = regexp.MustCompile(`^\d{4}(\d{2})(\d{2})-(\d{2})(\d{2})`)
var linkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
var codePattern = regexp.MustCompile("`([^`]+)`")
var boldPattern = regexp.MustCompile(`\*\*([^*]+)\*\*`)
var terminalWidth = func() *runewidth.Condition {
	condition := runewidth.NewCondition()
	// Modern terminals render East Asian Ambiguous characters such as arrows
	// and box-drawing glyphs as one cell, independently of the process locale.
	condition.EastAsianWidth = false
	return condition
}()

func paint(code, value string) string {
	if !useColor || value == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func dim(value string) string  { return paint("90", value) }
func bold(value string) string { return paint("1", value) }

func statusColor(status string) string {
	switch status {
	case "INVESTIGATE":
		return "35"
	case "READY":
		return "32"
	case "DOING":
		return "33"
	case "VERIFY":
		return "36"
	case "APPLIED":
		return "1;32"
	case "BLOCKED":
		return "31"
	case "VALIDATED":
		return "92"
	case "REJECTED":
		return "90"
	default:
		return "0"
	}
}

func priorityColor(priority string) string {
	switch priority {
	case "P0":
		return "1;31"
	case "P1":
		return "33"
	case "P2":
		return "36"
	default:
		return "90"
	}
}

func targetStatusColor(status string) string {
	switch status {
	case "ACTIVE":
		return "1;31"
	case "RESOLVED":
		return "92"
	default:
		return "90"
	}
}

func width(value string) int {
	return terminalWidth.StringWidth(value)
}

func pad(value string, size int) string {
	if extra := size - width(value); extra > 0 {
		return value + strings.Repeat(" ", extra)
	}
	return value
}

func truncate(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if width(value) <= max {
		return value
	}
	ellipsis := "…"
	ellipsisWidth := width(ellipsis)
	if max < ellipsisWidth {
		return strings.Repeat(".", max)
	}
	return terminalWidth.Truncate(value, max, ellipsis)
}

func shortRun(run string) string {
	match := runPattern.FindStringSubmatch(run)
	if match == nil {
		return run
	}
	return match[1] + "/" + match[2] + " " + match[3] + ":" + match[4]
}

func orValue(value, fallback string) string {
	if value == "" || value == "none" {
		return fallback
	}
	return value
}

func countLabel(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}

type listLayout struct {
	titleWidth    int
	relationWidth int
	idWidth       int
	targetWidth   int
	ownerWidth    int
}

func makeListLayout(cards []Card, targets []Target, wide int, showTargetStatus bool, objectives ...Objective) listLayout {
	layout := listLayout{idWidth: 5, targetWidth: 1, ownerWidth: 1}
	maxTitleWidth := 1
	for _, card := range cards {
		layout.idWidth = max(layout.idWidth, width(card.ID))
		maxTitleWidth = max(maxTitleWidth, width(card.Title))
		layout.targetWidth = max(layout.targetWidth, width(cardTargetMarker(card)))
		layout.ownerWidth = max(layout.ownerWidth, min(width(orValue(card.Owner, "-")), 28))
		layout.relationWidth = max(layout.relationWidth, width(cardRelation(card)))
	}
	for _, target := range targets {
		layout.idWidth = max(layout.idWidth, width(target.ID))
		layout.targetWidth = max(layout.targetWidth, width(targetObjectiveMarker(target)))
		maxTitleWidth = max(maxTitleWidth, width(target.Title))
		layout.relationWidth = max(layout.relationWidth, width(targetRelation(target, showTargetStatus)))
	}
	for _, objective := range objectives {
		layout.idWidth = max(layout.idWidth, width(objective.ID))
	}
	if layout.relationWidth > 24 {
		layout.relationWidth = 24
	}
	cardMetadataWidth := layout.ownerWidth
	fixedWidth := width("   P0  ") + layout.idWidth + layout.targetWidth + 2 + width(" │ ") + cardMetadataWidth
	if layout.relationWidth > 0 {
		fixedWidth += layout.relationWidth + width(" │ ")
	}
	availableTitleWidth := wide - fixedWidth
	if availableTitleWidth < 1 {
		availableTitleWidth = 1
	}
	layout.titleWidth = min(maxTitleWidth, availableTitleWidth)
	return layout
}

func cardTargetMarker(card Card) string {
	if len(card.ActiveTargetIDs) == 0 {
		return "-"
	}
	return "[" + strings.Join(card.ActiveTargetIDs, ",") + "]"
}

func targetObjectiveMarker(target Target) string {
	if len(target.ObjectiveIDs) == 0 {
		return ""
	}
	return "[" + strings.Join(target.ObjectiveIDs, ",") + "]"
}

func cardRelation(card Card) string {
	var parts []string
	if dependency := strings.TrimSpace(compactDependencySummary(card)); dependency != "" {
		parts = append(parts, dependency)
	}
	return strings.Join(parts, " ")
}

func targetRelation(target Target, showStatus bool) string {
	var parts []string
	if showStatus {
		parts = append(parts, "["+target.Status+"]")
	}
	if len(target.CardIDs) > 0 {
		parts = append(parts, "cards:"+strings.Join(target.CardIDs, ","))
	}
	return strings.Join(parts, " ")
}

func cardLine(card Card, wide int) string {
	return cardLineWithLayout(card, wide, makeListLayout([]Card{card}, nil, wide, false))
}

func cardLineWithLayout(card Card, wide int, layout listLayout) string {
	owner := pad(truncate(orValue(card.Owner, "-"), layout.ownerWidth), layout.ownerWidth)
	targetMarker := pad(cardTargetMarker(card), layout.targetWidth)
	prefix := fmt.Sprintf("  %s %s  %s  ", bold(pad(card.ID, layout.idWidth)), paint(priorityColor(card.Priority), pad(orValue(card.Priority, "-"), 2)), dim(targetMarker))
	title := pad(truncate(card.Title, layout.titleWidth), layout.titleWidth)
	metadata := dim(owner)
	relationText := cardRelation(card)
	if relationText == "" {
		return prefix + title + dim(" │ ") + metadata
	}
	relation := pad(truncate(relationText, layout.relationWidth), layout.relationWidth)
	return prefix + title + dim(" │ ") + metadata + dim(" │ ") + dim(relation)
}

func compactDependencySummary(card Card) string {
	var dependsOn, unblocks []string
	for _, dependency := range card.Dependencies {
		dependsOn = append(dependsOn, dependency.DependsOnCardID)
	}
	for _, dependency := range card.Unblocks {
		unblocks = append(unblocks, dependency.CardID)
	}
	var parts []string
	if len(dependsOn) > 0 {
		parts = append(parts, "dep:"+strings.Join(dependsOn, ","))
	}
	if len(unblocks) > 0 {
		parts = append(parts, "unblocks:"+strings.Join(unblocks, ","))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, " ")
}

func printList(cards []Card, targets []Target, objectives []Objective, revision int, nextID, nextTargetID, nextObjectiveID string, wide int, all bool) {
	fmt.Printf("%s  %s\n", bold("ISUCON Backlog"), dim(fmt.Sprintf("rev %d · next %s/%s/%s · %s · %s · %s", revision, orValue(nextID, "?"), orValue(nextTargetID, "?"), orValue(nextObjectiveID, "?"), countLabel(len(objectives), "objective"), countLabel(len(cards), "intervention"), countLabel(len(targets), "target"))))
	fmt.Println()
	layout := makeListLayout(cards, targets, wide, false, objectives...)
	printObjectiveListWithLayout(objectives, layout)
	printTargetListWithLayout(targets, wide, false, layout)
	groups := map[string][]Card{}
	for _, card := range cards {
		groups[card.Status] = append(groups[card.Status], card)
	}
	for _, status := range statusOrder {
		group := groups[status]
		if len(group) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			leftTargeted, rightTargeted := len(group[i].ActiveTargetIDs) > 0, len(group[j].ActiveTargetIDs) > 0
			if leftTargeted != rightTargeted {
				return leftTargeted
			}
			left, right := orValue(group[i].Priority, "P9"), orValue(group[j].Priority, "P9")
			if left != right {
				return left < right
			}
			return group[i].ID < group[j].ID
		})
		fmt.Printf("\n%s %s\n", paint(statusColor(status), status), dim(fmt.Sprintf("(%d)", len(group))))
		for _, card := range group {
			fmt.Println(cardLineWithLayout(card, wide, layout))
		}
	}
	if len(cards) == 0 {
		fmt.Println("\n" + dim("該当するカードがない"))
		return
	}
	mode := "active"
	if all {
		mode = "all"
	}
	fmt.Printf("\n%s\n", dim(fmt.Sprintf("%s · %s · 詳細は `task backlog -- intervention show %s`", countLabel(len(cards), "intervention"), mode, cards[0].ID)))
}

func printCard(card Card) {
	fmt.Printf("%s %s %s\n", bold(card.ID), paint(statusColor(card.Status), "["+card.Status+"]"), bold(card.Title))
	if card.Closed {
		fmt.Println(dim("(closed)"))
	}
	fmt.Println()
	metadata := []struct{ name, value string }{
		{"Card version", fmt.Sprintf("%d", card.Version)},
		{"Objectives", strings.Join(card.ObjectiveIDs, ", ")},
		{"Targets", formatTargetRelations(card.TargetIDs, card.TargetRoles)}, {"Primary target", card.PrimaryTargetID},
		{"Priority", card.Priority}, {"Owner", card.Owner}, {"Area", card.Area}, {"Source RUNs", card.SourceRuns},
		{"Compare RUNs", card.CompareRun}, {"Observed RUNs", card.ObservedRuns},
		{"Depends on", formatDependencies(card.Dependencies, false)}, {"Unblocks", formatDependencies(card.Unblocks, true)},
		{"Updated", card.Updated}, {"Updated by", card.UpdatedBy},
	}
	visibleMetadata := metadata[:0]
	for _, field := range metadata {
		if strings.TrimSpace(field.value) != "" {
			visibleMetadata = append(visibleMetadata, field)
		}
	}
	maxName := 0
	for _, field := range visibleMetadata {
		if len(field.name) > maxName {
			maxName = len(field.name)
		}
	}
	for _, field := range visibleMetadata {
		fmt.Printf("  %s  %s\n", dim(pad(field.name+":", maxName+1)), field.value)
	}
	for _, section := range card.Sections {
		fmt.Printf("\n%s\n", paint("1;36", section.Name))
		for _, line := range strings.Split(section.Body, "\n") {
			fmt.Println("  " + plainLine(line))
		}
	}
	if len(card.TargetAssessments) > 0 {
		fmt.Printf("\n%s\n", paint("1;36", "Target assessments"))
		ids := make([]string, 0, len(card.TargetAssessments))
		for id := range card.TargetAssessments {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Printf("  %s  %s\n", bold(id), card.TargetAssessments[id])
		}
	}
	if len(card.History) > 0 {
		fmt.Printf("\n%s\n", paint("1;36", sectionHistory))
		for _, entry := range card.History {
			line := entry.Raw
			if line == "" {
				line = fmt.Sprintf("- %s [%s] %s", entry.OccurredAt, entry.Actor, entry.Body)
			}
			fmt.Println("  " + plainLine(line))
		}
	}
}

func printObjectiveList(objectives []Objective) {
	printObjectiveListWithLayout(objectives, makeListLayout(nil, nil, defaultListWidth, false, objectives...))
}

func printObjectiveListWithLayout(objectives []Objective, layout listLayout) {
	if len(objectives) == 0 {
		fmt.Println("no objectives")
		return
	}
	fmt.Printf("%s %s\n", paint("1;34", "OBJECTIVES"), dim(fmt.Sprintf("(%d)", len(objectives))))
	for _, objective := range objectives {
		fmt.Printf("  %s %s  %s\n", bold(pad(objective.ID, layout.idWidth)), strings.Repeat(" ", layout.targetWidth+4), objective.Title)
	}
}

func printObjective(objective Objective) {
	fmt.Printf("%s %s %s %s\n\n", bold(objective.ID), paint("34", "["+objective.Status+"]"), objective.Mode, bold(objective.Title))
	metadata := []struct{ name, value string }{
		{"Objective version", fmt.Sprintf("%d", objective.Version)},
		{"Required for valid result", fmt.Sprintf("%t", objective.RequiredForValidResult)},
		{"Parent objective", objective.ParentObjectiveID},
		{"Official sources", objective.OfficialSources},
		{"Targets", strings.Join(objective.TargetIDs, ", ")},
		{"Interventions", strings.Join(objective.InterventionIDs, ", ")},
		{"Updated", objective.Updated},
		{"Updated by", objective.UpdatedBy},
	}
	for _, field := range metadata {
		if strings.TrimSpace(field.value) != "" {
			fmt.Printf("  %-27s %s\n", field.name+":", field.value)
		}
	}
	fmt.Printf("\n%s\n  %s\n", paint("1;36", "Metric or predicate"), plainLine(objective.MetricOrPredicate))
	fmt.Printf("\n%s\n  %s\n", paint("1;36", "Verification"), plainLine(objective.Verification))
	if len(objective.History) > 0 {
		fmt.Printf("\n%s\n", paint("1;36", sectionHistory))
		for _, entry := range objective.History {
			fmt.Printf("  %s\n", plainLine(fmt.Sprintf("- %s [%s] %s", entry.OccurredAt, entry.Actor, entry.Body)))
		}
	}
}

func printTargetList(targets []Target, wide int, showStatus bool) {
	printTargetListWithLayout(targets, wide, showStatus, makeListLayout(nil, targets, wide, showStatus))
}

func printTargetListWithLayout(targets []Target, wide int, showStatus bool, layout listLayout) {
	if len(targets) == 0 {
		return
	}
	fmt.Printf("\n%s %s\n", paint("1;31", "TARGETS"), dim(fmt.Sprintf("(%d)", len(targets))))
	for _, target := range targets {
		fmt.Println(targetLineWithLayout(target, showStatus, layout))
	}
}

func targetLineWithLayout(target Target, showStatus bool, layout listLayout) string {
	prefix := fmt.Sprintf("  %s %s  %s  ", bold(pad(target.ID, layout.idWidth)), paint(priorityColor(target.Priority), pad(orValue(target.Priority, "-"), 2)), dim(pad(targetObjectiveMarker(target), layout.targetWidth)))
	title := pad(truncate(target.Title, layout.titleWidth), layout.titleWidth)
	relationText := truncate(targetRelation(target, showStatus), layout.relationWidth)
	relation := dim(relationText)
	if showStatus && relationText != "" {
		status := "[" + target.Status + "]"
		relation = paint(targetStatusColor(target.Status), status) + dim(strings.TrimPrefix(relationText, status))
	}
	return prefix + title + dim(" │ ") + relation
}

func printTarget(target Target) {
	fmt.Printf("%s %s %s\n\n", bold(target.ID), paint(targetStatusColor(target.Status), "["+target.Status+"]"), bold(target.Title))
	metadata := []struct{ name, value string }{
		{"Target version", fmt.Sprintf("%d", target.Version)},
		{"Objectives", strings.Join(target.ObjectiveIDs, ", ")},
		{"Priority", target.Priority},
		{"Fingerprint", target.Fingerprint},
		{"Scope", target.Scope}, {"Axis", target.Axis}, {"Goal", target.Goal}, {"Evaluation", target.Evaluation}, {"Previous target", target.PreviousTargetID}, {"Primary objective", target.PrimaryObjectiveID}, {"Completion evidence", target.CompletionEvidence},
		{"Source RUNs", target.SourceRuns},
		{"Observed RUNs", target.ObservedRuns},
		{"Merged into", target.MergedIntoID},
		{"Linked interventions", formatTargetRelations(target.CardIDs, target.CardRoles)},
		{"Updated", target.Updated},
		{"Updated by", target.UpdatedBy},
	}
	maxName := 0
	for _, field := range metadata {
		if len(field.name) > maxName {
			maxName = len(field.name)
		}
	}
	for _, field := range metadata {
		if strings.TrimSpace(field.value) != "" {
			fmt.Printf("  %s  %s\n", dim(pad(field.name+":", maxName+1)), field.value)
		}
	}
	fmt.Printf("\n%s\n  %s\n", paint("1;36", "Evidence"), plainLine(target.Evidence))
	fmt.Printf("\n%s\n  %s\n", paint("1;36", "Legacy resolution condition"), plainLine(target.Resolution))
	for _, id := range target.ObjectiveIDs {
		if rationale := target.ObjectiveRationales[id]; rationale != "" {
			fmt.Printf("\nObjective %s contribution: %s\n", id, plainLine(rationale))
		}
	}
	if len(target.History) > 0 {
		fmt.Printf("\n%s\n", paint("1;36", sectionHistory))
		for _, entry := range target.History {
			fmt.Printf("  %s\n", plainLine(fmt.Sprintf("- %s [%s] %s", entry.OccurredAt, entry.Actor, entry.Body)))
		}
	}
}

func formatTargetRelations(ids []string, roles map[string]string) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		value := id
		if role := roles[id]; role != "" {
			value += " [" + role + "]"
		}
		values = append(values, value)
	}
	return strings.Join(values, ", ")
}

func formatDependencies(dependencies []CardDependency, reverse bool) string {
	values := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		id := dependency.DependsOnCardID
		status := dependency.TargetStatus
		if reverse {
			id = dependency.CardID
			status = dependency.TargetStatus
		}
		value := fmt.Sprintf("%s [%s requires %s; current %s]", id, dependency.Mode, dependency.RequiredStatus, status)
		if strings.TrimSpace(dependency.Reason) != "" {
			value += " — " + dependency.Reason
		}
		values = append(values, value)
	}
	return strings.Join(values, "\n")
}

func inlineStyle(value string) string {
	value = linkPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := linkPattern.FindStringSubmatch(match)
		return parts[1] + dim(" ("+parts[2]+")")
	})
	value = codePattern.ReplaceAllStringFunc(value, func(match string) string { return paint("36", strings.Trim(match, "`")) })
	value = boldPattern.ReplaceAllStringFunc(value, func(match string) string { return bold(strings.Trim(match, "*")) })
	return value
}

func plainLine(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]
	if strings.HasPrefix(trimmed, "- ") {
		return indent + dim("-") + " " + inlineStyle(strings.TrimPrefix(trimmed, "- "))
	}
	return indent + inlineStyle(trimmed)
}

func initColor(noColor bool) {
	stat, _ := os.Stdout.Stat()
	isTTY := stat != nil && stat.Mode()&os.ModeCharDevice != 0
	useColor = !noColor && os.Getenv("NO_COLOR") == "" && (isTTY || os.Getenv("FORCE_COLOR") != "")
}
