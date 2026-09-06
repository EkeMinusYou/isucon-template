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

func constraintStatusColor(status string) string {
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
	titleWidth      int
	relationWidth   int
	idWidth         int
	constraintWidth int
	ownerWidth      int
}

func makeListLayout(cards []Card, constraints []Constraint, wide int, showConstraintStatus bool) listLayout {
	layout := listLayout{idWidth: 5, constraintWidth: 1, ownerWidth: 1}
	maxTitleWidth := 1
	for _, card := range cards {
		layout.idWidth = max(layout.idWidth, width(card.ID))
		maxTitleWidth = max(maxTitleWidth, width(card.Title))
		layout.constraintWidth = max(layout.constraintWidth, width(cardConstraintMarker(card)))
		layout.ownerWidth = max(layout.ownerWidth, min(width(orValue(card.Owner, "-")), 28))
		layout.relationWidth = max(layout.relationWidth, width(cardRelation(card)))
	}
	for _, constraint := range constraints {
		layout.idWidth = max(layout.idWidth, width(constraint.ID))
		maxTitleWidth = max(maxTitleWidth, width(constraint.Title))
		layout.relationWidth = max(layout.relationWidth, width(constraintRelation(constraint, showConstraintStatus)))
	}
	if layout.relationWidth > 24 {
		layout.relationWidth = 24
	}
	cardMetadataWidth := layout.ownerWidth
	fixedWidth := width("   P0  ") + layout.idWidth + layout.constraintWidth + 2 + width(" │ ") + cardMetadataWidth
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

func cardConstraintMarker(card Card) string {
	if len(card.ActiveConstraintIDs) == 0 {
		return "-"
	}
	return "[" + strings.Join(card.ActiveConstraintIDs, ",") + "]"
}

func cardRelation(card Card) string {
	var parts []string
	if dependency := strings.TrimSpace(compactDependencySummary(card)); dependency != "" {
		parts = append(parts, dependency)
	}
	return strings.Join(parts, " ")
}

func constraintRelation(constraint Constraint, showStatus bool) string {
	var parts []string
	if showStatus {
		parts = append(parts, "["+constraint.Status+"]")
	}
	if len(constraint.CardIDs) > 0 {
		parts = append(parts, "cards:"+strings.Join(constraint.CardIDs, ","))
	}
	return strings.Join(parts, " ")
}

func cardLine(card Card, wide int) string {
	return cardLineWithLayout(card, wide, makeListLayout([]Card{card}, nil, wide, false))
}

func cardLineWithLayout(card Card, wide int, layout listLayout) string {
	owner := pad(truncate(orValue(card.Owner, "-"), layout.ownerWidth), layout.ownerWidth)
	constraintMarker := pad(cardConstraintMarker(card), layout.constraintWidth)
	prefix := fmt.Sprintf("  %s %s  %s  ", bold(pad(card.ID, layout.idWidth)), paint(priorityColor(card.Priority), pad(orValue(card.Priority, "-"), 2)), dim(constraintMarker))
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

func printList(cards []Card, constraints []Constraint, objectives []Objective, revision int, nextID, nextConstraintID, nextObjectiveID string, wide int, all bool) {
	fmt.Printf("%s  %s\n", bold("ISUCON Backlog"), dim(fmt.Sprintf("rev %d · next %s/%s/%s · %s · %s · %s", revision, orValue(nextID, "?"), orValue(nextConstraintID, "?"), orValue(nextObjectiveID, "?"), countLabel(len(objectives), "objective"), countLabel(len(cards), "intervention"), countLabel(len(constraints), "constraint"))))
	fmt.Println()
	printObjectiveList(objectives)
	layout := makeListLayout(cards, constraints, wide, false)
	printConstraintListWithLayout(constraints, wide, false, layout)
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
			leftConstrainted, rightConstrainted := len(group[i].ActiveConstraintIDs) > 0, len(group[j].ActiveConstraintIDs) > 0
			if leftConstrainted != rightConstrainted {
				return leftConstrainted
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
		{"Constraints", formatConstraintRelations(card.ConstraintIDs, card.ConstraintRoles)},
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
	if len(card.ConstraintAssessments) > 0 {
		fmt.Printf("\n%s\n", paint("1;36", "Constraint assessments"))
		ids := make([]string, 0, len(card.ConstraintAssessments))
		for id := range card.ConstraintAssessments {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Printf("  %s  %s\n", bold(id), card.ConstraintAssessments[id])
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
	if len(objectives) == 0 {
		fmt.Println("no objectives")
		return
	}
	fmt.Printf("%s %s\n", paint("1;34", "OBJECTIVES"), dim(fmt.Sprintf("(%d)", len(objectives))))
	for _, objective := range objectives {
		required := ""
		if objective.RequiredForValidResult {
			required = " required"
		}
		fmt.Printf("  %s %-8s %-8s %s%s\n", bold(objective.ID), "["+objective.Status+"]", objective.Mode, objective.Title, dim(required))
	}
}

func printObjective(objective Objective) {
	fmt.Printf("%s %s %s %s\n\n", bold(objective.ID), paint("34", "["+objective.Status+"]"), objective.Mode, bold(objective.Title))
	metadata := []struct{ name, value string }{
		{"Objective version", fmt.Sprintf("%d", objective.Version)},
		{"Required for valid result", fmt.Sprintf("%t", objective.RequiredForValidResult)},
		{"Parent objective", objective.ParentObjectiveID},
		{"Official sources", objective.OfficialSources},
		{"Constraints", strings.Join(objective.ConstraintIDs, ", ")},
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

func printConstraintList(constraints []Constraint, wide int, showStatus bool) {
	printConstraintListWithLayout(constraints, wide, showStatus, makeListLayout(nil, constraints, wide, showStatus))
}

func printConstraintListWithLayout(constraints []Constraint, wide int, showStatus bool, layout listLayout) {
	if len(constraints) == 0 {
		return
	}
	fmt.Printf("\n%s %s\n", paint("1;31", "CONSTRAINTS"), dim(fmt.Sprintf("(%d)", len(constraints))))
	for _, constraint := range constraints {
		fmt.Println(constraintLineWithLayout(constraint, showStatus, layout))
	}
}

func constraintLineWithLayout(constraint Constraint, showStatus bool, layout listLayout) string {
	prefix := fmt.Sprintf("  %s %s  %s", bold(pad(constraint.ID, layout.idWidth)), paint(priorityColor(constraint.Priority), pad(orValue(constraint.Priority, "-"), 2)), strings.Repeat(" ", layout.constraintWidth+2))
	title := pad(truncate(constraint.Title, layout.titleWidth), layout.titleWidth)
	relationText := truncate(constraintRelation(constraint, showStatus), layout.relationWidth)
	relation := dim(relationText)
	if showStatus && relationText != "" {
		status := "[" + constraint.Status + "]"
		relation = paint(constraintStatusColor(constraint.Status), status) + dim(strings.TrimPrefix(relationText, status))
	}
	return prefix + title + dim(" │ ") + relation
}

func printConstraint(constraint Constraint) {
	fmt.Printf("%s %s %s\n\n", bold(constraint.ID), paint(constraintStatusColor(constraint.Status), "["+constraint.Status+"]"), bold(constraint.Title))
	metadata := []struct{ name, value string }{
		{"Constraint version", fmt.Sprintf("%d", constraint.Version)},
		{"Objectives", strings.Join(constraint.ObjectiveIDs, ", ")},
		{"Priority", constraint.Priority},
		{"Fingerprint", constraint.Fingerprint},
		{"Scope", constraint.Scope},
		{"Source RUNs", constraint.SourceRuns},
		{"Observed RUNs", constraint.ObservedRuns},
		{"Merged into", constraint.MergedIntoID},
		{"Linked interventions", formatConstraintRelations(constraint.CardIDs, constraint.CardRoles)},
		{"Updated", constraint.Updated},
		{"Updated by", constraint.UpdatedBy},
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
	fmt.Printf("\n%s\n  %s\n", paint("1;36", "Evidence"), plainLine(constraint.Evidence))
	fmt.Printf("\n%s\n  %s\n", paint("1;36", "Resolution condition"), plainLine(constraint.Resolution))
	if len(constraint.History) > 0 {
		fmt.Printf("\n%s\n", paint("1;36", sectionHistory))
		for _, entry := range constraint.History {
			fmt.Printf("  %s\n", plainLine(fmt.Sprintf("- %s [%s] %s", entry.OccurredAt, entry.Actor, entry.Body)))
		}
	}
}

func formatConstraintRelations(ids []string, roles map[string]string) string {
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
