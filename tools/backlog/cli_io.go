package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type outputFormat string

func (f *outputFormat) String() string { return string(*f) }

func (f *outputFormat) Set(value string) error {
	if value != "text" && value != "json" {
		return fmt.Errorf("format must be text or json, got %q", value)
	}
	*f = outputFormat(value)
	return nil
}

func outputFormatFlag(fs *flag.FlagSet) *outputFormat {
	format := outputFormat("text")
	fs.Var(&format, "format", "output format: text or json")
	return &format
}

func printJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fatal(err)
	}
}

type cardMutationReceipt struct {
	ID      string   `json:"id"`
	Version int      `json:"version"`
	Woke    []string `json:"woke"`
}

func printCardMutation(format outputFormat, id string, version int, message string, woke []string) {
	if format == "json" {
		// Use the version committed by this operation, not a later concurrent write.
		printJSON(cardMutationReceipt{ID: id, Version: version, Woke: append([]string{}, woke...)})
		return
	}
	fmt.Println(message)
	for _, id := range woke {
		fmt.Printf("woke %s: BLOCKED -> INVESTIGATE\n", id)
	}
}

type resultInput struct {
	text *string
	file *string
}

func resultInputFlags(fs *flag.FlagSet) resultInput {
	return resultInput{
		text: fs.String("result", "", "replace the Result section with text"),
		file: fs.String("result-file", "", "read Result text from a file, or - for stdin"),
	}
}

func (input resultInput) read(fs *flag.FlagSet, stdin io.Reader) (*string, error) {
	hasText, hasFile := flagSeen(fs, "result"), flagSeen(fs, "result-file")
	if hasText && hasFile {
		return nil, errors.New("--result and --result-file are mutually exclusive")
	}
	if !hasText && !hasFile {
		return nil, nil
	}
	body := *input.text
	if hasFile {
		var data []byte
		var err error
		if *input.file == "-" {
			data, err = io.ReadAll(stdin)
		} else {
			data, err = os.ReadFile(*input.file)
		}
		if err != nil {
			return nil, fmt.Errorf("read --result-file: %w", err)
		}
		body = string(data)
	}
	body = strings.TrimRight(body, "\n")
	return &body, nil
}

type sectionInput struct {
	stdin  *bool
	result resultInput
}

func sectionInputFlags(fs *flag.FlagSet) sectionInput {
	return sectionInput{
		stdin:  fs.Bool("section-stdin", false, "read section bodies as a JSON object from stdin"),
		result: resultInputFlags(fs),
	}
}

func (input sectionInput) read(fs *flag.FlagSet, stdin io.Reader) (map[string]string, error) {
	if *input.stdin && flagSeen(fs, "result-file") && *input.result.file == "-" {
		return nil, errors.New("--section-stdin and --result-file - cannot share stdin")
	}
	result, err := input.result.read(fs, stdin)
	if err != nil {
		return nil, err
	}
	var sections map[string]string
	if *input.stdin {
		sections, err = readSectionStdin(stdin)
		if err != nil {
			return nil, err
		}
	}
	if result != nil {
		if _, exists := sections["Result"]; exists {
			return nil, errors.New("Result supplied both by --section-stdin and --result/--result-file")
		}
		if sections == nil {
			sections = map[string]string{}
		}
		sections["Result"] = *result
	}
	return sections, nil
}
