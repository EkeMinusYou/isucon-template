package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const staticTTL = 24 * time.Hour
const staticCacheValidation = 60 * time.Second
const staticCacheEntries = 1024
const staticCacheBytes = 64 << 20
const staticMaxFileBytes = 8 << 20

type staticAsset struct {
	data, compressed            []byte
	etag, gzipETag, contentType string
	modtime, validated, used    time.Time
}
type staticHandler struct {
	root  string
	mu    sync.Mutex
	files map[string]*staticAsset
	bytes int
}

func newStaticHandler(root string) (*staticHandler, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("STATIC_DIR is not a directory: %s", root)
	}
	return &staticHandler{root: absolute, files: make(map[string]*staticAsset)}, nil
}
func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." || strings.HasPrefix(filepath.Base(name), ".") {
		http.NotFound(w, r)
		return
	}
	asset, file, err := h.load(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if file != nil {
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		staticHeaders(w, time.Now())
		http.ServeContent(w, r, name, info.ModTime(), file)
		return
	}
	data, etag := asset.data, asset.etag
	if asset.compressed != nil {
		w.Header().Add("Vary", "Accept-Encoding")
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			data, etag = asset.compressed, asset.gzipETag
			w.Header().Set("Content-Encoding", "gzip")
		}
	}
	staticHeaders(w, time.Now())
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", asset.contentType)
	http.ServeContent(w, r, name, asset.modtime, bytes.NewReader(data))
}
func staticHeaders(w http.ResponseWriter, now time.Time) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Expires", now.Add(staticTTL).UTC().Format(http.TimeFormat))
}
func (h *staticHandler) load(name string) (*staticAsset, *os.File, error) {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if a := h.files[name]; a != nil && now.Sub(a.validated) < staticCacheValidation {
		a.used = now
		return a, nil, nil
	}
	full := filepath.Join(h.root, filepath.FromSlash(name))
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return nil, nil, err
	}
	relative, err := filepath.Rel(h.root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, nil, os.ErrPermission
	}
	f, err := os.Open(resolved)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, nil, os.ErrNotExist
	}
	if info.Size() > staticMaxFileBytes {
		return nil, f, nil
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, nil, err
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	sum := sha256.Sum256(data)
	asset := &staticAsset{data: data, etag: fmt.Sprintf(`"%x"`, sum), contentType: contentType, modtime: info.ModTime(), validated: now, used: now}
	kind, _, _ := mime.ParseMediaType(contentType)
	if len(data) >= 1024 && (kind == "text/html" || kind == "text/css" || kind == "application/javascript" || kind == "text/javascript" || kind == "image/svg+xml") {
		var compressed bytes.Buffer
		g, _ := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
		g.Write(data)
		g.Close()
		if compressed.Len() < len(data) {
			asset.compressed = compressed.Bytes()
			asset.gzipETag = fmt.Sprintf(`"%x-gzip"`, sum)
		}
	}
	if old := h.files[name]; old != nil {
		h.bytes -= len(old.data) + len(old.compressed)
		delete(h.files, name)
	}
	size := len(asset.data) + len(asset.compressed)
	for len(h.files) > 0 && (len(h.files) >= staticCacheEntries || h.bytes+size > staticCacheBytes) {
		oldest := ""
		var last time.Time
		for key, a := range h.files {
			if oldest == "" || a.used.Before(last) {
				oldest, last = key, a.used
			}
		}
		a := h.files[oldest]
		h.bytes -= len(a.data) + len(a.compressed)
		delete(h.files, oldest)
	}
	h.files[name] = asset
	h.bytes += size
	return asset, nil, nil
}
func acceptsGzip(value string) bool {
	wildcard := false
	for _, part := range strings.Split(value, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		name := strings.ToLower(strings.TrimSpace(fields[0]))
		q := 1.0
		for _, p := range fields[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
			if ok && k == "q" {
				n, e := strconv.ParseFloat(v, 64)
				if e != nil || n < 0 || n > 1 {
					q = 0
				} else {
					q = n
				}
			}
		}
		if name == "gzip" {
			return q > 0
		}
		if name == "*" {
			wildcard = q > 0
		}
	}
	return wildcard
}
