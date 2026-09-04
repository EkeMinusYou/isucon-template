package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type cliConfig struct {
	root     string
	dbPath   string
	dumpPath string
	noColor  bool
	wide     int
}

const defaultListWidth = 140

func main() {
	config, args, err := parseGlobal(os.Args[1:])
	if err != nil {
		fatal(err)
	}
	initColor(config.noColor)
	command, commandArgs := resolveCommand(args)
	if command == "help" || command == "--help" || command == "-h" {
		usage()
		return
	}
	if config.dbPath == "" {
		config.dbPath = filepath.Join(config.root, defaultDatabase)
	}
	if config.dumpPath == "" && config.dbPath != ":memory:" {
		config.dumpPath = strings.TrimSuffix(config.dbPath, filepath.Ext(config.dbPath)) + ".sql"
	}
	if err := restoreDatabaseIfMissing(config.dbPath, config.dumpPath); err != nil {
		fatal(err)
	}

	switch command {
	case "init":
		runInit(config, commandArgs)
	case "list":
		runList(config, commandArgs)
	case "watch":
		runList(config, append([]string{"--watch"}, commandArgs...))
	case "show":
		runShow(config, commandArgs)
	case "intervention":
		runIntervention(config, commandArgs)
	case "snapshot-applied":
		runSnapshotApplied(config, commandArgs)
	case "add":
		runAdd(config, commandArgs)
	case "update":
		runUpdate(config, commandArgs)
	case "resolve":
		runResolve(config, commandArgs)
	case "transition", "close":
		runTransition(config, commandArgs)
	case "history":
		runHistory(config, commandArgs)
	case "dependency":
		runDependency(config, commandArgs)
	case "objective":
		runObjective(config, commandArgs)
	case "constraint", "anchor":
		runConstraint(config, commandArgs)
	case "pass":
		runPass(config, commandArgs)
	case "validate":
		runValidate(config, commandArgs)
	case "evidence":
		runEvidence(config, commandArgs)
	default:
		fatal(fmt.Errorf("unknown command %q; see --help", command))
	}
	if mutatesBacklog(command, commandArgs) {
		if err := dumpDatabase(config.dbPath, config.dumpPath); err != nil {
			fatal(err)
		}
	}
}

func parseGlobal(args []string) (cliConfig, []string, error) {
	config := cliConfig{root: ".", wide: defaultListWidth}
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		value := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", arg)
			}
			i++
			return args[i], nil
		}
		switch arg {
		case "-root", "--root":
			v, err := value()
			if err != nil {
				return config, nil, err
			}
			config.root = v
		case "-db", "--db":
			v, err := value()
			if err != nil {
				return config, nil, err
			}
			config.dbPath = v
		case "-dump", "--dump":
			v, err := value()
			if err != nil {
				return config, nil, err
			}
			config.dumpPath = v
		case "-w", "--width":
			v, err := value()
			if err != nil {
				return config, nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 20 {
				return config, nil, fmt.Errorf("invalid width %q", v)
			}
			config.wide = n
		case "-no-color", "--no-color":
			config.noColor = true
		default:
			rest = append(rest, args[i:]...)
			return config, rest, nil
		}
	}
	return config, rest, nil
}

func mutatesBacklog(command string, args []string) bool {
	switch command {
	case "init", "add", "update", "resolve", "transition", "close", "history", "pass":
		return true
	case "objective":
		return len(args) > 0 && args[0] != "list" && args[0] != "show"
	case "dependency":
		return len(args) > 0 && (args[0] == "add" || args[0] == "remove")
	case "constraint", "anchor":
		return len(args) > 0 && args[0] != "list" && args[0] != "show"
	default:
		return false
	}
}

func restoreDatabaseIfMissing(dbPath, dumpPath string) error {
	if dbPath == ":memory:" || dumpPath == "" {
		return nil
	}
	if _, err := os.Stat(dbPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat backlog database: %w", err)
	}
	if _, err := os.Stat(dumpPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat backlog dump: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create backlog database directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dbPath), ".backlog-restore-*.sqlite3")
	if err != nil {
		return fmt.Errorf("create temporary backlog database: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temporary backlog database: %w", err)
	}
	defer os.Remove(tmpPath)

	dump, err := os.Open(dumpPath)
	if err != nil {
		return fmt.Errorf("open backlog dump: %w", err)
	}
	defer dump.Close()
	cmd := exec.Command("sqlite3", tmpPath)
	cmd.Stdin = dump
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restore backlog database with sqlite3: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(tmpPath, dbPath); err != nil {
		return fmt.Errorf("install restored backlog database: %w", err)
	}
	fmt.Printf("restored backlog: %s from %s\n", dbPath, dumpPath)
	return nil
}

func dumpDatabase(dbPath, dumpPath string) error {
	if dbPath == ":memory:" || dumpPath == "" {
		return nil
	}
	cmd := exec.Command("sqlite3", dbPath, ".dump")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("dump backlog database with sqlite3: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := os.MkdirAll(filepath.Dir(dumpPath), 0o755); err != nil {
		return fmt.Errorf("create backlog dump directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dumpPath), ".backlog-dump-*.sql")
	if err != nil {
		return fmt.Errorf("create temporary backlog dump: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(output); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary backlog dump: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary backlog dump: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set backlog dump permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary backlog dump: %w", err)
	}
	if err := os.Rename(tmpPath, dumpPath); err != nil {
		return fmt.Errorf("install backlog dump: %w", err)
	}
	return nil
}

func resolveCommand(args []string) (string, []string) {
	if len(args) == 0 {
		return "list", nil
	}
	if strings.HasPrefix(args[0], "-") {
		return "list", args
	}
	if strings.HasPrefix(strings.ToUpper(args[0]), "B-") || isDigits(args[0]) {
		return "show", args
	}
	if strings.HasPrefix(strings.ToUpper(args[0]), "A-") {
		return "constraint", append([]string{"show"}, args...)
	}
	if strings.HasPrefix(strings.ToUpper(args[0]), "O-") {
		return "objective", append([]string{"show"}, args...)
	}
	return strings.ToLower(args[0]), args[1:]
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func openCLIStore(config cliConfig) *Store {
	store, err := openStore(config.dbPath)
	if err != nil {
		fatal(err)
	}
	return store
}

func runInit(config cliConfig, args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.Parse(args)
	store := openCLIStore(config)
	defer store.Close()
	if err := store.reconcileNextCardID(); err != nil {
		fatal(err)
	}
	fmt.Printf("initialized backlog: %s\n", config.dbPath)
}

func runList(config cliConfig, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	all := fs.Bool("all", false, "include terminal cards")
	status := fs.String("status", "", "filter by status")
	area := fs.String("area", "", "filter by area")
	priority := fs.String("p", "", "filter by priority")
	owner := fs.String("owner", "", "filter by exact owner")
	constraintID := fs.String("constraint", "", "filter by linked constraint")
	unowned := fs.Bool("unowned", false, "filter cards without an owner")
	watch := fs.Bool("watch", false, "refresh the list periodically")
	interval := fs.Duration("interval", 15*time.Second, "refresh interval in watch mode")
	noColor := fs.Bool("no-color", false, "disable terminal colors")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if *noColor {
		initColor(true)
	}
	if *watch && *interval <= 0 {
		fatal(errors.New("--interval must be greater than 0"))
	}
	if *unowned && *owner != "" {
		fatal(errors.New("--unowned and --owner cannot be used together"))
	}
	store := openCLIStore(config)
	defer store.Close()
	filter := ListFilter{
		All: *all, Status: *status, Area: *area, Priority: *priority,
		Owner: *owner, ConstraintID: normalizeConstraintID(*constraintID), Unowned: *unowned,
	}
	render := func() error {
		cards, err := store.listCards(filter)
		if err != nil {
			return err
		}
		revision, err := store.revision()
		if err != nil {
			return err
		}
		nextID, err := store.metadata("next_id")
		if err != nil {
			return err
		}
		nextConstraintID, err := store.metadata("next_constraint_id")
		if err != nil {
			return err
		}
		nextObjectiveID, err := store.metadata("next_objective_id")
		if err != nil {
			return err
		}
		objectives, err := store.listObjectives(ObjectiveFilter{All: *all})
		if err != nil {
			return err
		}
		constraints, err := store.listConstraints(ConstraintFilter{All: *all})
		if err != nil {
			return err
		}
		printList(cards, constraints, objectives, revision, nextID, nextConstraintID, nextObjectiveID, config.wide, *all)
		return nil
	}
	if !*watch {
		if err := render(); err != nil {
			fatal(err)
		}
		return
	}

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	for {
		clearTerminal()
		fmt.Printf("%s\n\n", dim(fmt.Sprintf("Backlog watch · refresh every %s · %s", interval.String(), time.Now().Format("2006-01-02 15:04:05"))))
		if err := render(); err != nil {
			fatal(err)
		}
		select {
		case <-ticker.C:
		case <-interrupt:
			return
		}
	}
}

func clearTerminal() {
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Print("\033[2J\033[H")
}

func runShow(config cliConfig, args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	noColor := fs.Bool("no-color", false, "disable terminal colors")
	fs.Parse(args)
	if *noColor {
		initColor(true)
	}
	if fs.NArg() != 1 {
		fatal(errors.New("show requires one card ID"))
	}
	store := openCLIStore(config)
	defer store.Close()
	card, err := store.getCard(normalizeID(fs.Arg(0)))
	if err != nil {
		fatal(err)
	}
	printCard(card)
}

func runIntervention(config cliConfig, args []string) {
	if len(args) == 0 || strings.EqualFold(args[0], "list") {
		if len(args) > 0 {
			args = args[1:]
		}
		runList(config, args)
		return
	}
	if strings.EqualFold(args[0], "show") {
		runShow(config, args[1:])
		return
	}
	fatal(fmt.Errorf("unknown intervention subcommand %q", args[0]))
}

func runAdd(config cliConfig, args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	id := fs.String("id", "", "explicit card ID")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "history/change reason")
	sectionStdin := fs.Bool("section-stdin", false, "read section bodies as a JSON object from stdin")
	fieldFlags := cardFieldFlags(fs)
	title := fieldFlags.values["title"]
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if *title == "" {
		fatal(errors.New("--title is required"))
	}
	sections := readSectionsFromStdin(*sectionStdin)
	values := visitedFieldValues(fs, fieldFlags)
	store := openCLIStore(config)
	defer store.Close()
	newID, err := store.addCard(NewCard{ID: *id, Title: *title, Actor: *actor, Reason: *reason, CardPatch: CardPatch{Values: values, Sections: sections}}, mutation{Actor: *actor, Operation: "add"})
	if err != nil {
		fatal(err)
	}
	fmt.Println(newID)
}

type fieldFlagSet struct{ values map[string]*string }

func cardValueFlags() map[string]bool {
	result := map[string]bool{"actor": true, "reason": true, "expect-card-version": true}
	for _, key := range []string{"status", "title", "priority", "owner", "area", "source-runs", "compare-run", "observed-runs", "fingerprint", "updated", "updated-by"} {
		result[key] = true
	}
	return result
}

func cardFieldFlags(fs *flag.FlagSet) fieldFlagSet {
	values := map[string]*string{}
	for _, key := range []string{"status", "title", "priority", "owner", "area", "source-runs", "compare-run", "observed-runs", "fingerprint", "updated", "updated-by"} {
		value := ""
		fs.StringVar(&value, key, "", "card field")
		values[key] = &value
	}
	return fieldFlagSet{values: values}
}

func visitedFieldValues(fs *flag.FlagSet, fields fieldFlagSet) map[string]string {
	result := map[string]string{}
	fs.Visit(func(flag *flag.Flag) {
		if value, ok := fields.values[flag.Name]; ok {
			result[flag.Name] = *value
		}
	})
	return result
}

func readSectionsFromStdin(enabled bool) map[string]string {
	if !enabled {
		return nil
	}
	sections, err := readSectionStdin(os.Stdin)
	if err != nil {
		fatal(err)
	}
	return sections
}

func readSectionStdin(reader io.Reader) (map[string]string, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read --section-stdin: %w", err)
	}

	var input map[string]string
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, fmt.Errorf("invalid --section-stdin JSON object: %w", err)
	}
	if input == nil {
		return nil, errors.New("--section-stdin requires a JSON object")
	}
	if len(input) == 0 {
		return nil, errors.New("--section-stdin requires at least one section")
	}

	sections := make(map[string]string, len(input))
	for name, sectionBody := range input {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, errors.New("--section-stdin contains an empty section name")
		}
		if err := validateSectionName(name); err != nil {
			return nil, err
		}
		sections[name] = strings.TrimRight(sectionBody, "\n")
	}
	return sections, nil
}

func runUpdate(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, cardValueFlags())
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "history/change reason")
	expected := fs.Int("expect-card-version", -1, "expected target card version")
	sectionStdin := fs.Bool("section-stdin", false, "read section bodies as a JSON object from stdin")
	fields := cardFieldFlags(fs)
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 {
		fatal(errors.New("update requires one card ID"))
	}
	if *expected < 0 {
		fatal(errors.New("--expect-card-version is required for update"))
	}
	patch := CardPatch{Values: visitedFieldValues(fs, fields), Sections: readSectionsFromStdin(*sectionStdin)}
	store := openCLIStore(config)
	defer store.Close()
	cardID := normalizeID(fs.Arg(0))
	if err := store.updateCard(cardID, patch, mutation{ExpectedCardVersion: expected, Actor: *actor, Operation: "update"}, *reason); err != nil {
		fatal(err)
	}
	fmt.Println("updated")
}

func runResolve(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, cardValueFlags())
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "history/change reason")
	expected := fs.Int("expect-card-version", -1, "expected target card version")
	sectionStdin := fs.Bool("section-stdin", false, "read section bodies as a JSON object from stdin")
	fields := cardFieldFlags(fs)
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 {
		fatal(errors.New("resolve requires one card ID"))
	}
	if *expected < 0 {
		fatal(errors.New("--expect-card-version is required for resolve"))
	}
	values := visitedFieldValues(fs, fields)
	status, ok := values["status"]
	if !ok || strings.TrimSpace(status) == "" {
		fatal(errors.New("--status is required for resolve"))
	}
	patch := CardPatch{Values: values, Sections: readSectionsFromStdin(*sectionStdin)}
	store := openCLIStore(config)
	defer store.Close()
	cardID := normalizeID(fs.Arg(0))
	woke, err := store.resolveCardAndWake(cardID, status, patch, mutation{ExpectedCardVersion: expected, Actor: *actor, Operation: "resolve"}, *reason)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("resolved %s\n", normalizeStatus(status))
	for _, id := range woke {
		fmt.Printf("woke %s: BLOCKED -> INVESTIGATE\n", id)
	}
}

func flagSeen(fs *flag.FlagSet, name string) bool {
	seen := false
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == name {
			seen = true
		}
	})
	return seen
}

func moveCardIDToEnd(args []string, valueFlags map[string]bool) []string {
	var flags []string
	cardID := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
			if !strings.Contains(arg, "=") && valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		if cardID == "" {
			cardID = arg
		} else {
			flags = append(flags, arg)
		}
	}
	if cardID != "" {
		flags = append(flags, cardID)
	}
	return flags
}

func runTransition(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"status": true, "actor": true, "reason": true, "expect-card-version": true})
	fs := flag.NewFlagSet("transition", flag.ExitOnError)
	status := fs.String("status", "", "new status")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "history/change reason")
	expected := fs.Int("expect-card-version", -1, "expected target card version")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 {
		fatal(errors.New("transition requires one card ID"))
	}
	if *expected < 0 {
		fatal(errors.New("--expect-card-version is required for transition"))
	}
	store := openCLIStore(config)
	defer store.Close()
	cardID := normalizeID(fs.Arg(0))
	woke, err := store.transitionCardAndWake(cardID, *status, mutation{ExpectedCardVersion: expected, Actor: *actor, Operation: "transition"}, *reason)
	if err != nil {
		fatal(err)
	}
	fmt.Println("transitioned")
	for _, id := range woke {
		fmt.Printf("woke %s: BLOCKED -> INVESTIGATE\n", id)
	}
}

func runSnapshotApplied(config cliConfig, args []string) {
	fs := flag.NewFlagSet("snapshot-applied", flag.ExitOnError)
	output := fs.String("output", "", "snapshot JSON output path")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 0 {
		fatal(errors.New("snapshot-applied accepts no positional arguments"))
	}
	store := openCLIStore(config)
	defer store.Close()
	snapshot, err := store.appliedSnapshot()
	if err != nil {
		fatal(err)
	}
	if err := writeAppliedSnapshot(*output, snapshot); err != nil {
		fatal(err)
	}
	fmt.Printf("snapshotted %d APPLIED cards at revision %d: %s\n", len(snapshot.Cards), snapshot.Revision, *output)
}

func runDependency(config cliConfig, args []string) {
	if len(args) == 0 {
		fatal(errors.New("dependency requires add, remove, or list"))
	}
	subcommand := strings.ToLower(args[0])
	switch subcommand {
	case "add":
		runDependencyAdd(config, args[1:])
	case "remove":
		runDependencyRemove(config, args[1:])
	case "list":
		runDependencyList(config, args[1:])
	default:
		fatal(fmt.Errorf("unknown dependency subcommand %q", subcommand))
	}
}

func runDependencyAdd(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"on": true, "required-status": true, "mode": true, "actor": true, "reason": true, "expect-card-version": true})
	fs := flag.NewFlagSet("dependency add", flag.ExitOnError)
	on := fs.String("on", "", "dependency target card ID")
	requiredStatus := fs.String("required-status", "", "VERIFY, APPLIED, or VALIDATED")
	mode := fs.String("mode", "", "ORDERING or BLOCKING")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "dependency reason")
	expected := fs.Int("expect-card-version", -1, "expected dependent card version")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 || *on == "" {
		fatal(errors.New("dependency add requires CARD_ID and --on CARD_ID"))
	}
	if *expected < 0 {
		fatal(errors.New("--expect-card-version is required for dependency add"))
	}
	store := openCLIStore(config)
	defer store.Close()
	woke, err := store.addDependency(DependencyInput{CardID: fs.Arg(0), DependsOnCardID: *on, RequiredStatus: *requiredStatus, Mode: *mode, Reason: *reason},
		mutation{ExpectedCardVersion: expected, Actor: *actor, Operation: "dependency.add"})
	if err != nil {
		fatal(err)
	}
	fmt.Println("dependency added")
	if woke {
		fmt.Printf("woke %s: BLOCKED -> INVESTIGATE\n", normalizeID(fs.Arg(0)))
	}
}

func runDependencyRemove(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"on": true, "actor": true, "reason": true, "expect-card-version": true})
	fs := flag.NewFlagSet("dependency remove", flag.ExitOnError)
	on := fs.String("on", "", "dependency target card ID")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "removal reason")
	expected := fs.Int("expect-card-version", -1, "expected dependent card version")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 || *on == "" {
		fatal(errors.New("dependency remove requires CARD_ID and --on CARD_ID"))
	}
	if *expected < 0 {
		fatal(errors.New("--expect-card-version is required for dependency remove"))
	}
	store := openCLIStore(config)
	defer store.Close()
	woke, err := store.removeDependency(fs.Arg(0), *on, mutation{ExpectedCardVersion: expected, Actor: *actor, Operation: "dependency.remove"}, *reason)
	if err != nil {
		fatal(err)
	}
	fmt.Println("dependency removed")
	if woke {
		fmt.Printf("woke %s: BLOCKED -> INVESTIGATE\n", normalizeID(fs.Arg(0)))
	}
}

func runDependencyList(config cliConfig, args []string) {
	fs := flag.NewFlagSet("dependency list", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 {
		fatal(errors.New("dependency list requires one card ID"))
	}
	store := openCLIStore(config)
	defer store.Close()
	card, err := store.getCard(normalizeID(fs.Arg(0)))
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Depends on:\n%s\n", formatDependencies(card.Dependencies, false))
	fmt.Printf("Unblocks:\n%s\n", formatDependencies(card.Unblocks, true))
}

func runObjective(config cliConfig, args []string) {
	if len(args) == 0 {
		runObjectiveList(config, nil)
		return
	}
	switch strings.ToLower(args[0]) {
	case "list":
		runObjectiveList(config, args[1:])
	case "show":
		runObjectiveShow(config, args[1:])
	case "add":
		runObjectiveAdd(config, args[1:])
	case "update":
		runObjectiveUpdate(config, args[1:])
	case "link", "unlink":
		runObjectiveRelation(config, args[0], args[1:])
	default:
		fatal(fmt.Errorf("unknown objective subcommand %q", args[0]))
	}
}

func runObjectiveList(config cliConfig, args []string) {
	fs := flag.NewFlagSet("objective list", flag.ExitOnError)
	all := fs.Bool("all", false, "include retired objectives")
	status := fs.String("status", "", "filter by objective status")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	store := openCLIStore(config)
	defer store.Close()
	objectives, err := store.listObjectives(ObjectiveFilter{All: *all, Status: *status})
	if err != nil {
		fatal(err)
	}
	printObjectiveList(objectives)
}

func runObjectiveShow(config cliConfig, args []string) {
	fs := flag.NewFlagSet("objective show", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 {
		fatal(errors.New("objective show requires one objective ID"))
	}
	store := openCLIStore(config)
	defer store.Close()
	objective, err := store.getObjective(fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	printObjective(objective)
}

func runObjectiveAdd(config cliConfig, args []string) {
	fs := flag.NewFlagSet("objective add", flag.ExitOnError)
	id := fs.String("id", "", "explicit objective ID")
	status := fs.String("status", "ACTIVE", "ACTIVE or RETIRED")
	mode := fs.String("mode", "", "SATISFY, MAXIMIZE, or MINIMIZE")
	title := fs.String("title", "", "objective title")
	metric := fs.String("metric-or-predicate", "", "metric or predicate used to judge the objective")
	required := fs.Bool("required-for-valid-result", false, "mark as required for a valid result")
	parent := fs.String("parent-objective", "", "parent objective ID")
	sources := fs.String("official-sources", "", "comma-separated official source paths")
	verification := fs.String("verification", "", "how the objective is verified")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "creation reason")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	store := openCLIStore(config)
	defer store.Close()
	newID, err := store.addObjective(NewObjective{ID: *id, Status: *status, Mode: *mode, Title: *title, MetricOrPredicate: *metric, RequiredForValidResult: *required, ParentObjectiveID: *parent, OfficialSources: *sources, Verification: *verification}, mutation{Actor: *actor, Operation: "objective.add"}, *reason)
	if err != nil {
		fatal(err)
	}
	fmt.Println(newID)
}

func runObjectiveUpdate(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"expect-objective-version": true, "status": true, "mode": true, "title": true, "metric-or-predicate": true, "required-for-valid-result": true, "parent-objective": true, "official-sources": true, "verification": true, "actor": true, "reason": true})
	fs := flag.NewFlagSet("objective update", flag.ExitOnError)
	expected := fs.Int("expect-objective-version", -1, "expected objective version")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "update reason")
	values := map[string]*string{}
	for _, name := range []string{"status", "mode", "title", "metric-or-predicate", "required-for-valid-result", "parent-objective", "official-sources", "verification"} {
		value := ""
		fs.StringVar(&value, name, "", "objective field")
		values[name] = &value
	}
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 || *expected < 0 {
		fatal(errors.New("objective update requires one objective ID and --expect-objective-version"))
	}
	patch := ObjectivePatch{}
	var parseErr error
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "status":
			patch.Status = values[f.Name]
		case "mode":
			patch.Mode = values[f.Name]
		case "title":
			patch.Title = values[f.Name]
		case "metric-or-predicate":
			patch.MetricOrPredicate = values[f.Name]
		case "parent-objective":
			patch.ParentObjectiveID = values[f.Name]
		case "official-sources":
			patch.OfficialSources = values[f.Name]
		case "verification":
			patch.Verification = values[f.Name]
		case "required-for-valid-result":
			value, err := strconv.ParseBool(*values[f.Name])
			if err != nil {
				parseErr = fmt.Errorf("invalid required-for-valid-result: %w", err)
				return
			}
			patch.RequiredForValidResult = &value
		}
	})
	if parseErr != nil {
		fatal(parseErr)
	}
	store := openCLIStore(config)
	defer store.Close()
	if err := store.updateObjective(fs.Arg(0), patch, *expected, mutation{Actor: *actor, Operation: "objective.update"}, *reason); err != nil {
		fatal(err)
	}
	fmt.Println("objective updated")
}

func runObjectiveRelation(config cliConfig, action string, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"constraint": true, "intervention": true, "rationale": true, "expect-objective-version": true, "actor": true, "reason": true})
	fs := flag.NewFlagSet("objective "+action, flag.ExitOnError)
	constraintID := fs.String("constraint", "", "constraint ID")
	interventionID := fs.String("intervention", "", "intervention card ID")
	rationale := fs.String("rationale", "", "causal rationale for an intervention relation")
	expected := fs.Int("expect-objective-version", -1, "expected objective version")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "relation reason")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 || *expected < 0 || (*constraintID == "") == (*interventionID == "") {
		fatal(errors.New("objective link/unlink requires one objective ID, exactly one of --constraint or --intervention, and --expect-objective-version"))
	}
	targetType, targetID := "constraint", *constraintID
	if *interventionID != "" {
		targetType, targetID = "intervention", *interventionID
	}
	store := openCLIStore(config)
	defer store.Close()
	if err := store.setObjectiveRelation(fs.Arg(0), targetID, targetType, *rationale, action == "link", *expected, mutation{Actor: *actor, Operation: "objective." + action}, *reason); err != nil {
		fatal(err)
	}
	if action == "link" {
		fmt.Println("objective relation linked")
	} else {
		fmt.Println("objective relation unlinked")
	}
}

func runConstraint(config cliConfig, args []string) {
	if len(args) == 0 {
		runConstraintList(config, nil)
		return
	}
	switch strings.ToLower(args[0]) {
	case "list":
		runConstraintList(config, args[1:])
	case "show":
		runConstraintShow(config, args[1:])
	case "add":
		runConstraintAdd(config, args[1:])
	case "update":
		runConstraintUpdate(config, args[1:])
	case "transition":
		runConstraintTransition(config, args[1:])
	case "assess":
		runConstraintAssess(config, args[1:])
	case "link", "unlink":
		runConstraintLink(config, args[0], args[1:])
	default:
		fatal(fmt.Errorf("unknown constraint subcommand %q", args[0]))
	}
}

func runConstraintList(config cliConfig, args []string) {
	fs := flag.NewFlagSet("constraint list", flag.ExitOnError)
	all := fs.Bool("all", false, "include terminal constraints")
	status := fs.String("status", "", "filter by constraint status")
	priority := fs.String("p", "", "filter by priority")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	store := openCLIStore(config)
	defer store.Close()
	constraints, err := store.listConstraints(ConstraintFilter{All: *all, Status: *status, Priority: *priority})
	if err != nil {
		fatal(err)
	}
	printConstraintList(constraints, config.wide, true)
	if len(constraints) == 0 {
		fmt.Println("no constraints")
	}
}

func runConstraintShow(config cliConfig, args []string) {
	fs := flag.NewFlagSet("constraint show", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 {
		fatal(errors.New("constraint show requires one constraint ID"))
	}
	store := openCLIStore(config)
	defer store.Close()
	constraint, err := store.getConstraint(fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	printConstraint(constraint)
}

func runConstraintAdd(config cliConfig, args []string) {
	fs := flag.NewFlagSet("constraint add", flag.ExitOnError)
	id := fs.String("id", "", "explicit constraint ID")
	status := fs.String("status", "ACTIVE", "new constraints must be ACTIVE")
	title := fs.String("title", "", "constraint title")
	priority := fs.String("priority", "", "constraint priority")
	objectiveID := fs.String("objective", "", "ACTIVE objective constrained by this fact")
	fingerprint := fs.String("fingerprint", "", "stable problem fingerprint")
	scope := fs.String("scope", "", "affected subject, path, loop, or validity condition")
	sourceRuns := fs.String("source-runs", "", "source RUN IDs")
	observedRuns := fs.String("observed-runs", "", "observed RUN IDs")
	evidence := fs.String("evidence", "", "absolute evidence with unit and denominator")
	resolution := fs.String("resolution", "", "condition that resolves the constraint")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "creation reason")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if strings.TrimSpace(*objectiveID) == "" {
		fatal(errors.New("constraint add requires --objective"))
	}
	store := openCLIStore(config)
	defer store.Close()
	objective, err := store.getObjective(*objectiveID)
	if err != nil {
		fatal(err)
	}
	if objective.Status != "ACTIVE" {
		fatal(fmt.Errorf("constraint requires an ACTIVE objective; %s is %s", objective.ID, objective.Status))
	}
	newID, err := store.addConstraint(NewConstraint{ID: *id, ObjectiveID: objective.ID, Status: *status, Title: *title, Priority: *priority, Fingerprint: *fingerprint,
		Scope: *scope, SourceRuns: *sourceRuns, ObservedRuns: *observedRuns,
		Evidence: *evidence, Resolution: *resolution}, mutation{Actor: *actor, Operation: "constraint.add"}, *reason)
	if err != nil {
		fatal(err)
	}
	fmt.Println(newID)
}

func runConstraintUpdate(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"expect-constraint-version": true, "actor": true, "reason": true, "title": true, "priority": true, "fingerprint": true, "scope": true, "source-runs": true, "observed-runs": true, "evidence": true, "resolution": true})
	fs := flag.NewFlagSet("constraint update", flag.ExitOnError)
	expected := fs.Int("expect-constraint-version", -1, "expected constraint version")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "update reason")
	values := map[string]*string{}
	for _, name := range []string{"title", "priority", "fingerprint", "scope", "source-runs", "observed-runs", "evidence", "resolution"} {
		value := ""
		fs.StringVar(&value, name, "", "constraint field")
		values[name] = &value
	}
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 || *expected < 0 {
		fatal(errors.New("constraint update requires one constraint ID and --expect-constraint-version"))
	}
	patch := ConstraintPatch{}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "title":
			patch.Title = values[f.Name]
		case "priority":
			patch.Priority = values[f.Name]
		case "fingerprint":
			patch.Fingerprint = values[f.Name]
		case "scope":
			patch.Scope = values[f.Name]
		case "source-runs":
			patch.SourceRuns = values[f.Name]
		case "observed-runs":
			patch.ObservedRuns = values[f.Name]
		case "evidence":
			patch.Evidence = values[f.Name]
		case "resolution":
			patch.Resolution = values[f.Name]
		}
	})
	store := openCLIStore(config)
	defer store.Close()
	if err := store.updateConstraint(fs.Arg(0), patch, *expected, mutation{Actor: *actor, Operation: "constraint.update"}, *reason); err != nil {
		fatal(err)
	}
	fmt.Println("constraint updated")
}

func runConstraintTransition(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"expect-constraint-version": true, "status": true, "merged-into": true, "actor": true, "reason": true})
	fs := flag.NewFlagSet("constraint transition", flag.ExitOnError)
	expected := fs.Int("expect-constraint-version", -1, "expected constraint version")
	status := fs.String("status", "", "target constraint status")
	mergedInto := fs.String("merged-into", "", "surviving constraint ID for MERGED")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "transition reason")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 || *expected < 0 || *status == "" {
		fatal(errors.New("constraint transition requires one constraint ID, --expect-constraint-version, and --status"))
	}
	store := openCLIStore(config)
	defer store.Close()
	if err := store.transitionConstraint(fs.Arg(0), *status, *mergedInto, *expected, mutation{Actor: *actor, Operation: "constraint.transition"}, *reason); err != nil {
		fatal(err)
	}
	fmt.Println("constraint transitioned")
}

func runConstraintLink(config cliConfig, action string, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"card": true, "role": true, "assessment-json": true, "expect-constraint-version": true, "actor": true, "reason": true})
	fs := flag.NewFlagSet("constraint "+action, flag.ExitOnError)
	cardID := fs.String("card", "", "intervention card ID")
	role := fs.String("role", "", "RESOLVES or MITIGATES; may be inferred from a performance assessment")
	expected := fs.Int("expect-constraint-version", -1, "expected constraint version")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "link reason")
	assessmentJSON := fs.String("assessment-json", "", "optional structured performance residual assessment JSON")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 || *cardID == "" || *expected < 0 {
		fatal(errors.New("constraint link/unlink requires one constraint ID, --card, and --expect-constraint-version"))
	}
	store := openCLIStore(config)
	defer store.Close()
	if err := store.setConstraintLink(fs.Arg(0), *cardID, action == "link", *role, *assessmentJSON, *expected, mutation{Actor: *actor, Operation: "constraint." + action}, *reason); err != nil {
		fatal(err)
	}
	if action == "link" {
		fmt.Println("constraint linked")
	} else {
		fmt.Println("constraint unlinked")
	}
}

func runConstraintAssess(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"card": true, "assessment-json": true, "expect-constraint-version": true, "actor": true, "reason": true})
	fs := flag.NewFlagSet("constraint assess", flag.ExitOnError)
	cardID := fs.String("card", "", "intervention card ID")
	assessmentJSON := fs.String("assessment-json", "", "structured performance residual assessment JSON")
	expected := fs.Int("expect-constraint-version", -1, "expected constraint version")
	actor := fs.String("actor", "", "writer identity")
	reason := fs.String("reason", "", "assessment reason")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 || *cardID == "" || *assessmentJSON == "" || *expected < 0 {
		fatal(errors.New("constraint assess requires one constraint ID, --card, --assessment-json, and --expect-constraint-version"))
	}
	store := openCLIStore(config)
	defer store.Close()
	if err := store.setConstraintInterventionAssessment(fs.Arg(0), *cardID, *assessmentJSON, *expected, mutation{Actor: *actor, Operation: "constraint.assess"}, *reason); err != nil {
		fatal(err)
	}
	fmt.Println("constraint intervention assessed")
}

func runHistory(config cliConfig, args []string) {
	if len(args) == 0 || args[0] != "add" {
		fatal(errors.New("history requires the 'add' subcommand"))
	}
	args = moveCardIDToEnd(args[1:], map[string]bool{"actor": true, "message": true, "at": true})
	fs := flag.NewFlagSet("history add", flag.ExitOnError)
	actor := fs.String("actor", "", "writer identity")
	body := fs.String("message", "", "history message")
	at := fs.String("at", "", "event timestamp")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 {
		fatal(errors.New("history add requires one card ID"))
	}
	store := openCLIStore(config)
	defer store.Close()
	cardID := normalizeID(fs.Arg(0))
	if err := store.addHistory(cardID, *actor, *body, *at, mutation{Actor: *actor, Operation: "history.add"}); err != nil {
		fatal(err)
	}
	fmt.Println("history added")
}

func runValidate(config cliConfig, args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	fs.Parse(args)
	store := openCLIStore(config)
	defer store.Close()
	if err := store.validate(); err != nil {
		fatal(err)
	}
	count, err := store.cardCount()
	if err != nil {
		fatal(err)
	}
	revision, err := store.revision()
	if err != nil {
		fatal(err)
	}
	constraints, err := store.listConstraints(ConstraintFilter{All: true})
	if err != nil {
		fatal(err)
	}
	objectives, err := store.listObjectives(ObjectiveFilter{All: true})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("OK: backlog integrity, %d interventions, %d constraints, %d objectives, revision %d\n", count, len(constraints), len(objectives), revision)
}

func runPass(config cliConfig, args []string) {
	args = moveCardIDToEnd(args, map[string]bool{"actor": true, "reason": true, "evidence-run": true})
	fs := flag.NewFlagSet("pass", flag.ExitOnError)
	actor := fs.String("actor", "", "internal writer identity")
	reason := fs.String("reason", "benchmark passed; promote selected APPLIED cards", "history/change reason")
	evidenceRun := fs.String("evidence-run", "", "finalized benchmark RUN containing the APPLIED snapshot")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if fs.NArg() != 1 {
		fatal(errors.New("pass requires a comma-separated card ID list or all"))
	}
	if *actor != "task:pass" {
		fatal(errors.New("the backlog pass subcommand is internal; use top-level 'task pass' so runs/outcomes.tsv is recorded"))
	}
	if strings.TrimSpace(*evidenceRun) == "" {
		fatal(errors.New("--evidence-run is required for pass"))
	}
	store := openCLIStore(config)
	defer store.Close()
	run, err := loadRunAppliedSnapshot(config.root, *evidenceRun)
	if err != nil {
		fatal(err)
	}
	ids, err := passSnapshotCardIDs(store, run, fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	if len(ids) == 0 {
		revision, err := store.revision()
		if err != nil {
			fatal(err)
		}
		fmt.Printf("pass: 0 cards, revision %d\n", revision)
		return
	}
	passReason := fmt.Sprintf("%s (evidence_run=%s, snapshot_revision=%d)", strings.TrimSpace(*reason), run.RunID, run.BacklogSnapshot.Revision)
	cards, err := store.promoteAppliedCardsMatching(ids, snapshotDefinitionHashes(run), *actor, passReason)
	if err != nil {
		fatal(err)
	}
	for _, card := range cards {
		fmt.Printf("validated %s: %s\n", card.ID, card.Title)
	}
	if err := store.validate(); err != nil {
		fatal(err)
	}
	revision, err := store.revision()
	if err != nil {
		fatal(err)
	}
	fmt.Printf("pass: %d cards, revision %d\n", len(cards), revision)
}

func passSnapshotCardIDs(store *Store, run runSnapshotEnvelope, value string) ([]string, error) {
	ids := snapshotCardIDs(run)
	if !strings.EqualFold(strings.TrimSpace(value), "all") {
		requested, err := parseCardIDList(value)
		if err != nil {
			return nil, err
		}
		available := make(map[string]bool, len(ids))
		for _, id := range ids {
			available[id] = true
		}
		for _, id := range requested {
			if !available[id] {
				return nil, fmt.Errorf("card %s was not APPLIED in evidence RUN %s", id, run.RunID)
			}
		}
		ids = requested
	}
	for _, id := range ids {
		card, err := store.getCard(id)
		if err != nil {
			return nil, err
		}
		if card.Status != "APPLIED" {
			return nil, fmt.Errorf("card %s has status %s; expected APPLIED", id, card.Status)
		}
		if err := validateRunSnapshotCard(run, card); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func passCardIDs(store *Store, value string) ([]string, error) {
	if strings.EqualFold(strings.TrimSpace(value), "all") {
		cards, err := store.listCards(ListFilter{All: true, Status: "APPLIED"})
		if err != nil {
			return nil, fmt.Errorf("list APPLIED cards: %w", err)
		}
		ids := make([]string, 0, len(cards))
		for _, card := range cards {
			ids = append(ids, card.ID)
		}
		return ids, nil
	}
	return parseCardIDList(value)
}

func parseCardIDList(value string) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, errors.New("pass requires non-empty comma-separated card IDs")
		}
		id := normalizeID(item)
		if _, err := idNumber(id); err != nil {
			return nil, err
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func usage() {
	fmt.Print(`Backlog CLI

Global options:
  -root PATH                 repository root (default .)
  -db PATH                   backlog data path (default configured path)
  -dump PATH                 tracked SQL dump (default: DB path with .sql extension)
  -w, --width N              list display width (default 140)
  -no-color                  disable terminal colors

Commands:
  init                                    initialize the backlog
  list [--all] [--status S] [--area A] [-p P] [--constraint A-NNN] [--unowned|--owner ACTOR] [--watch] [--interval D]
  watch [list options]                   refresh the list periodically
  show CARD_ID
  snapshot-applied --output PATH          atomically save the current APPLIED card set
  add --title TITLE --actor ACTOR --reason REASON [card fields] [--section-stdin]
  update CARD_ID --expect-card-version N --actor ACTOR --reason REASON [card fields] [--section-stdin]
  resolve CARD_ID --expect-card-version N --status READY|BLOCKED|REJECTED --actor ACTOR --reason REASON [card fields] [--section-stdin]
  transition CARD_ID --expect-card-version N --status STATUS --actor ACTOR --reason REASON
  dependency add CARD_ID --on CARD_ID [--required-status STATUS --mode MODE] --expect-card-version N --actor ACTOR --reason REASON
  dependency remove CARD_ID --on CARD_ID --expect-card-version N --actor ACTOR --reason REASON
  dependency list CARD_ID
  objective list [--all] [--status S]
  objective show OBJECTIVE_ID
  objective add --mode MODE --title TITLE --metric-or-predicate VALUE --verification VALUE --actor ACTOR --reason REASON
  objective update OBJECTIVE_ID --expect-objective-version N --actor ACTOR --reason REASON [objective fields]
  objective link|unlink OBJECTIVE_ID (--constraint A-NNN|--intervention B-NNN) --expect-objective-version N --actor ACTOR --reason REASON
  constraint list [--all] [--status S] [-p P]
  constraint show CONSTRAINT_ID
  constraint add --objective O-NNN --title TITLE --fingerprint FP --scope SCOPE --evidence E --resolution R --actor ACTOR --reason REASON
  constraint update CONSTRAINT_ID --expect-constraint-version N --actor ACTOR --reason REASON [constraint fields]
  constraint transition CONSTRAINT_ID --expect-constraint-version N --status STATUS [--merged-into CONSTRAINT_ID] --actor ACTOR --reason REASON
  constraint assess CONSTRAINT_ID --card CARD_ID --assessment-json JSON --expect-constraint-version N --actor ACTOR --reason REASON
  constraint link CONSTRAINT_ID --card CARD_ID --role RESOLVES|MITIGATES [--assessment-json JSON] --expect-constraint-version N --actor ACTOR --reason REASON
  constraint unlink CONSTRAINT_ID --card CARD_ID --expect-constraint-version N --actor ACTOR --reason REASON
  history add CARD_ID --actor ACTOR --message MESSAGE
  pass CARD_ID[,CARD_ID...]|all --actor task:pass --evidence-run RUN_ID [--reason REASON]  internal command used by top-level 'task pass'
  evidence [-format text|json] CARD_ID[,CARD_ID...]  summarize normal benchmark evidence without writes
  validate

Every write transaction increments backlog_revision internally. update, resolve, and transition require
--expect-card-version so a changed target card is rejected instead of overwritten. History is append-only.
Card fields include normalized source/compare/observed RUN relations. Record purpose, boundary,
decision, safety, blockers, and reconsider conditions in the card sections and History.
`)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "backlog:", err)
	os.Exit(1)
}
