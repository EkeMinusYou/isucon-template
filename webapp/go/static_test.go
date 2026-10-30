package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticServingPreservesValidatorsCompressionHeadAndRange(t *testing.T) {
	root := t.TempDir()
	original := strings.Repeat("body { color: green; }\n", 100)
	os.WriteFile(filepath.Join(root, "app.css"), []byte(original), 0644)
	h, err := newStaticHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	defer client.CloseIdleConnections()
	get := func(method string, headers map[string]string) (*http.Response, []byte) {
		t.Helper()
		r, _ := http.NewRequest(method, server.URL+"/app.css", nil)
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		return response, data
	}
	response, plain := get("GET", nil)
	if response.StatusCode != 200 || string(plain) != original || response.Header.Get("Cache-Control") != "public, max-age=86400" || response.Header.Get("Expires") == "" {
		t.Fatalf("plain: %+v %q", response, plain)
	}
	etag := response.Header.Get("ETag")
	compressed, data := get("GET", map[string]string{"Accept-Encoding": "gzip"})
	if compressed.Header.Get("Content-Encoding") != "gzip" || compressed.Header.Get("ETag") == etag {
		t.Fatal("encoded representation was not independently validated")
	}
	reader, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := io.ReadAll(reader)
	reader.Close()
	if string(decoded) != original {
		t.Fatal("gzip body changed")
	}
	notModified, data := get("GET", map[string]string{"If-None-Match": etag})
	if notModified.StatusCode != 304 || len(data) != 0 {
		t.Fatal("conditional request failed")
	}
	head, data := get("HEAD", nil)
	if len(data) != 0 || head.ContentLength != int64(len(original)) {
		t.Fatal("HEAD changed representation metadata")
	}
	partial, data := get("GET", map[string]string{"Range": "bytes=0-3"})
	if partial.StatusCode != 206 || string(data) != original[:4] {
		t.Fatal("range failed")
	}
	identity, _ := get("GET", map[string]string{"Accept-Encoding": "gzip;q=0, *;q=1"})
	if identity.Header.Get("Content-Encoding") != "" {
		t.Fatal("explicit gzip rejection ignored")
	}
	response, data = get("POST", nil)
	if response.StatusCode != 405 {
		t.Fatalf("static mutation accepted: %d", response.StatusCode)
	}
}
func TestStaticHandlerDoesNotExposeDirectoriesOrOutsideSymlinks(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(secret, []byte("private"), 0600)
	os.Symlink(secret, filepath.Join(root, "outside"))
	h, _ := newStaticHandler(root)
	for _, name := range []string{"/", "/outside", "/.gitkeep"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", name, nil))
		if w.Code != 404 {
			t.Fatalf("exposed %s: %d", name, w.Code)
		}
	}
}
