package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Share the exact route classifier with transitions before feeding logs to alp.
// Raw logs remain unchanged; labels never contain unclassified API identifiers.
func normalizeAccessLog(cfg config, input io.Reader, output io.Writer) error {
	agg := newAggregator(cfg, 1)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maxLogLineBytes)
	writer := bufio.NewWriter(output)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &fields); err != nil {
			return fmt.Errorf("invalid access-log JSON")
		}
		var uri string
		if raw := bytes.TrimSpace(fields["uri"]); len(raw) == 0 || raw[0] != '"' {
			return fmt.Errorf("access-log URI must be a string")
		}
		if err := json.Unmarshal(fields["uri"], &uri); err != nil {
			return fmt.Errorf("access-log URI must be a string")
		}
		route, ok := agg.classify(uri)
		if !ok {
			route = normalizePath(uri)
			if strings.HasPrefix(route, cfg.APIPrefix) {
				route = "<unclassified-api>"
			}
		}
		fields["uri"], _ = json.Marshal(route)
		row, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(row, '\n')); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return writer.Flush()
}
