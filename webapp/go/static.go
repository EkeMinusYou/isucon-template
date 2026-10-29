package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type staticAsset struct {
	body, compressed                          []byte
	contentType, etag, gzipETag, cacheControl string
	modified                                  time.Time
}

// Immutable deployment assets are loaded once. Uploaded or mutable data needs
// its own handler; static data and gzip variants count against process memory.
func staticHandler(routes []staticRoute, next http.Handler) (http.Handler, error) {
	assets := map[string]staticAsset{}
	for _, route := range routes {
		if !strings.HasPrefix(route.Prefix, "/") || !strings.HasSuffix(route.Prefix, "/") || path.Clean(route.Prefix)+"/" != route.Prefix && route.Prefix != "/" || route.CacheControl == "" {
			return nil, fmt.Errorf("static route requires a clean /prefix/ and explicit cache_control")
		}
		err := filepath.WalkDir(route.Directory, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("static symlink is not allowed: %s", name)
			}
			if entry.IsDir() {
				if name != route.Directory && strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(name, ".gz") {
				return nil
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("static asset is not a regular file: %s", name)
			}
			rel, err := filepath.Rel(route.Directory, name)
			if err != nil {
				return err
			}
			url := route.Prefix + filepath.ToSlash(rel)
			if _, ok := assets[url]; ok {
				return fmt.Errorf("duplicate static URL: %s", url)
			}
			body, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			a := staticAsset{body: body, contentType: mime.TypeByExtension(filepath.Ext(name)), etag: fmt.Sprintf("\"%x\"", sha256.Sum256(body)), modified: info.ModTime(), cacheControl: route.CacheControl}
			if a.contentType == "" {
				a.contentType = http.DetectContentType(body)
			}
			a.compressed, err = os.ReadFile(name + ".gz")
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if len(a.compressed) > 0 {
				reader, err := gzip.NewReader(bytes.NewReader(a.compressed))
				if err != nil {
					return err
				}
				decoded, err := io.ReadAll(io.LimitReader(reader, int64(len(body))+1))
				reader.Close()
				if err != nil || !bytes.Equal(decoded, body) {
					return fmt.Errorf("stale or invalid gzip asset: %s", name)
				}
				a.gzipETag = fmt.Sprintf("\"%x\"", sha256.Sum256(a.compressed))
			}
			assets[url] = a
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := assets[r.URL.Path]
		if !ok || r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", a.contentType)
		w.Header().Set("Cache-Control", a.cacheControl)
		w.Header().Add("Vary", "Accept-Encoding")
		body, etag := a.body, a.etag
		if len(a.compressed) > 0 && r.Header.Get("Range") == "" && acceptsGzip(r.Header.Get("Accept-Encoding")) {
			body, etag = a.compressed, a.gzipETag
			w.Header().Set("Content-Encoding", "gzip")
		}
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, path.Base(r.URL.Path), a.modified, bytes.NewReader(body))
	}), nil
}

func acceptsGzip(value string) bool {
	wildcard := false
	for _, item := range strings.Split(value, ",") {
		fields := strings.Split(strings.TrimSpace(item), ";")
		name := strings.ToLower(strings.TrimSpace(fields[0]))
		q := 1.0
		for _, field := range fields[1:] {
			key, v, ok := strings.Cut(strings.TrimSpace(field), "=")
			if ok && strings.EqualFold(key, "q") {
				var err error
				q, err = strconv.ParseFloat(v, 64)
				if err != nil || q < 0 || q > 1 {
					q = 0
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
