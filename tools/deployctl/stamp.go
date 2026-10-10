package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Every successful upload leaves a stamp on its host: the content fingerprint of
// the local source and the worktree that sent it. Before a benchmark, verify
// recomputes the fingerprints in the measuring worktree, so a RUN never measures
// code another worktree deployed or local edits that were never deployed.
// Stamps live in the SSH user's home and need no sudo.
const stampDir = ".isucon-deploy"

type deployStamp struct {
	Remote      string `json:"remote"`
	Local       string `json:"local"`
	Fingerprint string `json:"fingerprint"`
	Worktree    string `json:"worktree"`
	Commit      string `json:"commit"`
	DeployedAt  string `json:"deployed_at"`
}

type provenance struct {
	once     sync.Once
	worktree string
	commit   string
}

func (p *provenance) get() (string, string) {
	p.once.Do(func() {
		p.worktree = gitOutput("rev-parse", "--show-toplevel")
		p.commit = gitOutput("rev-parse", "HEAD")
	})
	return p.worktree, p.commit
}

func gitOutput(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func stampKey(remote string) string {
	sum := sha256.Sum256([]byte(remote))
	return hex.EncodeToString(sum[:8])
}

func (r *deployRunner) stampScript(j uploadJob, fingerprint string) (string, error) {
	worktree, commit := r.provenance.get()
	body, err := json.Marshal(deployStamp{
		Remote: j.remote, Local: j.local, Fingerprint: fingerprint,
		Worktree: worktree, Commit: commit, DeployedAt: time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("set -eu\nmkdir -p \"$HOME/%s\"\ncat > \"$HOME/%s/%s.json\" <<'STAMP'\n%s\nSTAMP\n",
		stampDir, stampDir, stampKey(j.remote), body), nil
}

// localFingerprint hashes what rsync -a sends: relative paths, permission bits,
// file contents, and symlink targets, skipping the upload's excludes.
func localFingerprint(local string, excludes []string) (string, error) {
	root := strings.TrimSuffix(local, "/")
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	add := func(rel string, p string, info fs.FileInfo) error {
		fmt.Fprintf(h, "%s\x00%o\x00", rel, info.Mode()&(fs.ModeType|fs.ModePerm))
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			io.WriteString(h, target)
		case info.Mode().IsRegular():
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(h, f); err != nil {
				return err
			}
		}
		h.Write([]byte{0})
		return nil
	}
	if !info.IsDir() {
		if err := add(filepath.Base(root), root, info); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	var rels []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
		if excluded(rel, d.IsDir(), excludes) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rels = append(rels, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(rels)
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		if err := add(rel, p, info); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// excluded follows the rsync rules used by the declarations: a trailing slash
// matches directories only, a pattern with a slash matches the relative path,
// and any other pattern matches a name at any depth.
func excluded(rel string, dir bool, patterns []string) bool {
	for _, pattern := range patterns {
		dirOnly := strings.HasSuffix(pattern, "/")
		pattern = strings.TrimSuffix(pattern, "/")
		if dirOnly && !dir {
			continue
		}
		subject := path.Base(rel)
		if strings.Contains(pattern, "/") {
			subject = rel
			pattern = strings.TrimPrefix(pattern, "/")
		}
		if ok, _ := path.Match(pattern, subject); ok {
			return true
		}
	}
	return false
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	configPath := fs.String("config", "tools/deployctl/deployments.yaml", "deployment declaration")
	sshUser := fs.String("ssh-user", "ubuntu", "SSH user")
	sshOpts := fs.String("ssh-opts", "", "additional SSH options")
	_ = fs.String("run-state-file", "raw/current-run-id", "accepted for Taskfile common flags")
	parallel := fs.Int("parallel", 8, "maximum number of concurrent jobs")
	roles := roleValues{}
	vars := variableValues{}
	fs.Var(roles, "role", "role=host1,host2 (repeatable)")
	fs.Var(vars, "var", "name=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !nameRE.MatchString(*sshUser) {
		return fmt.Errorf("invalid SSH user: %q", *sshUser)
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	return newDeployRunner(*sshUser, *sshOpts, roles, vars, false, *parallel).verify(cfg)
}

type declaredUpload struct {
	local    string
	excludes []string
}

// verify compares every stamp on the hosts with the same declaration resolved in
// this worktree. Destinations without a stamp were never deployed by deployctl
// and are skipped.
func (r *deployRunner) verify(cfg *config) error {
	declared := map[string]map[string][]declaredUpload{}
	for _, name := range sortedDeploymentNames(cfg) {
		for _, u := range cfg.Deployments[name].Uploads {
			for _, host := range r.roles[u.Role] {
				e := expander{host: host, vars: r.vars}
				local, err := e.expand(u.Local)
				if err != nil {
					return fmt.Errorf("deployment %q upload %q local: %w", name, u.Label, err)
				}
				remote, err := e.expand(u.Remote)
				if err != nil {
					return fmt.Errorf("deployment %q upload %q remote: %w", name, u.Label, err)
				}
				if declared[host] == nil {
					declared[host] = map[string][]declaredUpload{}
				}
				declared[host][remote] = append(declared[host][remote], declaredUpload{local: local, excludes: u.Excludes})
			}
		}
	}
	hosts := make([]string, 0, len(declared))
	for host := range declared {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)

	worktree, _ := r.provenance.get()
	var mu sync.Mutex
	fingerprints := map[string]string{}
	fingerprint := func(u declaredUpload) (string, error) {
		key := u.local + "\x00" + strings.Join(u.excludes, "\x00")
		mu.Lock()
		defer mu.Unlock()
		if v, ok := fingerprints[key]; ok {
			return v, nil
		}
		v, err := localFingerprint(u.local, u.excludes)
		if err != nil {
			return "", err
		}
		fingerprints[key] = v
		return v, nil
	}

	results := runParallel(hosts, r.parallel, func(host string) jobResult {
		started := time.Now()
		result := func(output string, err error) jobResult {
			return jobResult{label: "deployed content matches this worktree", host: host, duration: time.Since(started), output: output, err: err}
		}
		args := append(r.sshBase(), r.sshTarget(host), "sh -s")
		out, err := r.exec.Run("ssh", args, fmt.Sprintf("cat \"$HOME/%s\"/*.json 2>/dev/null || true\n", stampDir))
		if err != nil {
			return result(strings.TrimSpace(string(out)), fmt.Errorf("ssh: %w", err))
		}
		var problems []string
		checked := 0
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var s deployStamp
			if err := json.Unmarshal([]byte(line), &s); err != nil {
				problems = append(problems, fmt.Sprintf("unreadable stamp: %v", err))
				continue
			}
			for _, u := range declared[host][s.Remote] {
				if u.local != s.Local {
					continue
				}
				checked++
				local, err := fingerprint(u)
				switch {
				case err != nil:
					problems = append(problems, fmt.Sprintf("%s: cannot read local %s: %v", s.Remote, s.Local, err))
				case local != s.Fingerprint:
					from := "this worktree"
					if s.Worktree != worktree {
						from = s.Worktree
					}
					problems = append(problems, fmt.Sprintf("%s differs from local %s (deployed from %s at commit %.12s, %s)", s.Remote, s.Local, from, s.Commit, s.DeployedAt))
				}
			}
		}
		if len(problems) != 0 {
			return result(strings.Join(problems, "\n"), fmt.Errorf("%d deployed destination(s) do not match this worktree; deploy from here before measuring", len(problems)))
		}
		return result(fmt.Sprintf("%d stamped destination(s) match", checked), nil)
	})
	if err := reportResults(results); err != nil {
		return err
	}
	return nil
}
