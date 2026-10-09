package main

import (
	"strings"
	"testing"
)

func TestParsePerfScript(t *testing.T) {
	input := ` nginx 10 [001] 1.000: cycles:
        aaa leaf+0x1 (/usr/sbin/nginx)
        bbb root+0x2 (/usr/sbin/nginx)

nginx 11 [000] 1.001: cycles:
        ccc [unknown] ([unknown])
        ddd root+0x2 (/usr/sbin/nginx)

`
	profile, err := parsePerfScript(strings.NewReader(input), 2)
	if err != nil {
		t.Fatalf("parsePerfScript: %v", err)
	}
	if profile.samples != 2 || profile.unknownSamples != 1 {
		t.Fatalf("samples=%d unknown=%d", profile.samples, profile.unknownSamples)
	}
	if got := profile.stacks["nginx;root+0x2;leaf+0x1"]; got != 1 {
		t.Fatalf("known stack count=%d", got)
	}
	if got := profile.stacks["nginx;root+0x2;[unknown]"]; got != 1 {
		t.Fatalf("unknown stack count=%d", got)
	}
}

func TestParsePerfScriptEnforcesSampleLimit(t *testing.T) {
	input := "nginx 1: cycles:\n  aaa leaf\n\nnginx 1: cycles:\n  aaa leaf\n\n"
	if _, err := parsePerfScript(strings.NewReader(input), 1); err == nil {
		t.Fatal("parsePerfScript accepted samples beyond the limit")
	}
}

func TestParseLostSamples(t *testing.T) {
	if got := parseLostSamples("lost 2 samples\nlost 3 samples"); got != 5 {
		t.Fatalf("parseLostSamples=%d, want 5", got)
	}
}
