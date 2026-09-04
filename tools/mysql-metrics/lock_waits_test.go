package main

import (
	"strings"
	"testing"
)

func TestLockWaitHeaderMatchesRowWidth(t *testing.T) {
	if got, want := len(lockWaitHeader()), 23; got != want {
		t.Fatalf("len(lockWaitHeader()) = %d, want %d", got, want)
	}
}

func TestSanitizeLockField(t *testing.T) {
	got := sanitizeLockField(" SELECT\t1\nFROM dual ")
	if got != "SELECT 1 FROM dual" {
		t.Fatalf("sanitizeLockField() = %q", got)
	}
	if got := sanitizeLockField(strings.Repeat("x", 5000)); len(got) != 4096 {
		t.Fatalf("sanitized length = %d, want 4096", len(got))
	}
}
