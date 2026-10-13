package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() { os.Exit(cli()) }

func cli() int {
	fs := flag.NewFlagSet("portal-bench", flag.ContinueOnError)
	path := fs.String("config", "", "Portal workflow JSON configuration (required)")
	base := fs.String("base-url", "", "Override the configured portal URL")
	output := fs.String("output-dir", os.Getenv("ISUCON_BENCH_ARTIFACT_DIR"), "Save benchmark-result.txt in this directory")
	jobID := fs.String("job-id", "", "Retrieve an existing job without submitting a new benchmark")
	check := fs.Bool("check", false, "Validate settings and credentials without requests or commands")
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *path == "" || fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "-config is required; positional arguments are not supported")
		return 2
	}
	c, err := loadConfig(*path)
	if err == nil {
		if *base != "" {
			c.BaseURL = *base
		}
		err = c.validate()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "portal-bench:", err)
		return 2
	}
	if *check {
		fmt.Fprintln(os.Stdout, "portal configuration is valid")
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = newRunner(c, os.Stdout, os.Stderr, *output).run(ctx, *jobID)
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, "portal-bench:", err)
	if errors.Is(err, errBenchmarkFailed) {
		return 1
	}
	return 2
}
