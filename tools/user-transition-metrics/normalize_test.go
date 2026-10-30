package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalNormalizationForAlpAndTransitions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	os.WriteFile(path, []byte(`{"cookie_field":"session_id","api_prefix":"/api/","routes":[
      {"name":"filtered","pattern":"^/api/search[?]mode=fast$","match_query":true},
      {"name":"search","pattern":"^/api/search$"},
      {"name":"item","pattern":"^/api/items/[^/]+$"}]}`), 0600)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	uris := []string{"/api/search?mode=fast", "/api/search?x=1", "/api/items/a%20b?x=2", "/api/unknown/secret?id=1", "/static/a.css?v=2"}
	want := []string{"filtered", "search", "item", "<unclassified-api>", "/static/a.css"}
	agg := newAggregator(cfg, 10)
	var input, output bytes.Buffer
	for i, uri := range uris {
		body, _ := json.Marshal(map[string]any{"uri": uri, "method": "GET", "status": 200, "body_bytes": 123})
		input.Write(append(body, '\n'))
		if i < 3 {
			route, ok := agg.classify(uri)
			if !ok || route != want[i] {
				t.Fatalf("classify %q = %q, %v", uri, route, ok)
			}
		}
	}
	if err := normalizeAccessLog(cfg, &input, &output); err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var got struct {
			URI  string `json:"uri"`
			Body int    `json:"body_bytes"`
		}
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatal(err)
		}
		if got.URI != want[i] || got.Body != 123 {
			t.Fatalf("row %d = %+v", i, got)
		}
	}
	if err := normalizeAccessLog(cfg, strings.NewReader("{\"uri\":null}\n"), &output); err == nil {
		t.Fatal("missing URI was silently accepted")
	}
	if err := normalizeAccessLog(cfg, strings.NewReader("broken\n"), &output); err == nil {
		t.Fatal("malformed JSON was silently accepted")
	}
}

func TestQueryClassificationTyposAreRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	os.WriteFile(path, []byte(`{"cookie_field":"session_id","api_prefix":"/api/","routes":[{"name":"search","pattern":"^/api/search$","match_qurey":true}]}`), 0600)
	if _, err := loadConfig(path); err == nil {
		t.Fatal("misspelled query policy was silently accepted")
	}
}
