package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run the remote scripts against an isolated local directory. No SSH or sudo
// privileges are used, and rsync only copies between temporary directories.
type guardedUploadExecutor struct {
	root            string
	transferFailure bool
	backupFailure   bool
	uploaded        bool
	activated       bool
}

func (f *guardedUploadExecutor) Run(name string, args []string, stdin string) ([]byte, error) {
	if name == "rsync" {
		f.uploaded = true
		source := args[len(args)-2]
		cmd := exec.Command("rsync", "-a", "--delete", source, filepath.Join(f.root, "live")+"/")
		output, err := cmd.CombinedOutput()
		if err == nil && f.transferFailure {
			err = errors.New("interrupted transfer")
		}
		return output, err
	}
	if name != "ssh" {
		return nil, errors.New("unexpected command")
	}
	if f.backupFailure && strings.Contains(stdin, "mkdir -m 0700") {
		return nil, errors.New("backup failed")
	}
	if strings.Contains(stdin, "echo activated") {
		f.activated = true
	}
	script := strings.ReplaceAll(stdin, "/etc/nginx", filepath.Join(f.root, "live"))
	script = strings.ReplaceAll(script, "/tmp/isucon-deployctl-", filepath.Join(f.root, "backups")+"/")
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(f.root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	return cmd.CombinedOutput()
}

func TestGuardedUploadRestoresFailedConfiguration(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync is not installed")
	}
	for _, tc := range []struct {
		name                                          string
		valid, transferFailure, absent, backupFailure bool
	}{
		{name: "valid", valid: true},
		{name: "invalid"},
		{name: "interrupted", valid: true, transferFailure: true},
		{name: "new destination", absent: true},
		{name: "backup failure", valid: true, backupFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			for _, dir := range []string{"source", "bin", "backups"} {
				if err := os.MkdirAll(filepath.Join(tmp, dir), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(tmp, "bin/sudo"), []byte("#!/bin/sh\nexec \"$@\"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			write := func(path, value string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(tmp, path), []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if !tc.absent {
				if err := os.Mkdir(filepath.Join(tmp, "live"), 0755); err != nil {
					t.Fatal(err)
				}
				write("live/config.conf", "old")
				write("live/removed.conf", "restore me")
			}
			value := "bad"
			if tc.valid {
				value = "good"
			}
			write("source/config.conf", value)
			write("source/added.conf", "new")
			fake := &guardedUploadExecutor{root: tmp, transferFailure: tc.transferFailure, backupFailure: tc.backupFailure}
			runner := deployRunner{exec: fake, parallel: 1, roles: map[string][]string{"nginx": {"host1"}}}
			// apply() validates repository-relative paths before any remote operation.
			previous, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(tmp); err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(previous)
			deployment := deployment{
				Uploads:     []upload{{Label: "config", Role: "nginx", Local: "source/", Remote: "/etc/nginx/", Delete: true, Validate: "test \"$(cat /etc/nginx/config.conf)\" = good"}},
				Activations: []activation{{Label: "activate", Role: "nginx", Script: "echo activated"}},
			}
			err = runner.apply(deployment)
			success := tc.valid && !tc.transferFailure && !tc.backupFailure
			if (err == nil) != success || fake.activated != success {
				t.Fatalf("err=%v activated=%v want success=%v", err, fake.activated, success)
			}
			if tc.backupFailure && fake.uploaded {
				t.Fatal("upload started before backup succeeded")
			}
			if !success && tc.absent {
				if _, err := os.Stat(filepath.Join(tmp, "live")); !os.IsNotExist(err) {
					t.Fatalf("new destination survived rollback: %v", err)
				}
			} else {
				expected := "old"
				if success {
					expected = "good"
				}
				body, err := os.ReadFile(filepath.Join(tmp, "live/config.conf"))
				if err != nil || string(body) != expected {
					t.Fatalf("configuration = %q, %v; want %q", body, err, expected)
				}
				for _, name := range []string{"added.conf", "removed.conf"} {
					_, err := os.Stat(filepath.Join(tmp, "live", name))
					shouldExist := (name == "added.conf") == success
					if (err == nil) != shouldExist {
						t.Fatalf("%s existence: %v", name, err)
					}
				}
			}
			backups, err := os.ReadDir(filepath.Join(tmp, "backups"))
			if err != nil || len(backups) != 0 {
				t.Fatalf("backup cleanup: %v %v", backups, err)
			}
		})
	}
}
