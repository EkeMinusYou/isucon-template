package main

import (
	"bufio"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type reportInfo struct {
	// Filename is the slash-separated path relative to docs/reports,
	// e.g. "isucon-analyze-alp/20260828-235318.md".
	Filename string `json:"filename"`
	// Kind is the directory the report lives in (the report type), or
	// "" for a report directly under docs/reports.
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	ModTime string `json:"mod_time"`
	Size    int64  `json:"size"`
}

// archiveDir holds reports written under the retired naming rules. They are
// kept as history and stay out of the dashboard listing.
const archiveDir = "archive"

func (a *app) listReports() ([]reportInfo, error) {
	reports := []reportInfo{}

	err := filepath.WalkDir(a.reportsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(a.reportsDir, p)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if rel == archiveDir {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".md") || name == "README.md" {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		reports = append(reports, reportInfo{
			Filename: rel,
			Kind:     kindOf(rel),
			Title:    firstHeading(p, rel),
			ModTime:  info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
			Size:     info.Size(),
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []reportInfo{}, nil
		}
		return nil, err
	}

	sort.Slice(reports, func(i, j int) bool { return reports[i].ModTime > reports[j].ModTime })
	return reports, nil
}

// kindOf returns the directory part of a report path relative to
// docs/reports, which is the report type it was filed under.
func kindOf(rel string) string {
	dir := path.Dir(rel)
	if dir == "." {
		return ""
	}
	return dir
}

// firstHeading reads the first "# " markdown heading from a file, falling
// back to the given default (the report path) when none is found.
func firstHeading(file, fallback string) string {
	f, err := os.Open(file)
	if err != nil {
		return fallback
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		return fallback
	}
	return fallback
}

func (a *app) handleReports(w http.ResponseWriter, r *http.Request) {
	reports, err := a.listReports()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, reports)
}

func (a *app) handleReportContent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	rel, ok := safeReportPath(name)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid report path")
		return
	}
	data, err := os.ReadFile(filepath.Join(a.reportsDir, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"filename": rel, "content": string(data)})
}

// safeReportPath rejects path traversal: the requested name must be a
// relative, cleaned, slash-separated ".md" path inside docs/reports.
func safeReportPath(name string) (string, bool) {
	if name == "" || !strings.HasSuffix(name, ".md") {
		return "", false
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return "", false
	}
	cleaned := path.Clean(name)
	if cleaned != name || cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return "", false
	}
	return cleaned, true
}
