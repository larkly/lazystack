package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/config"
)

func TestReportConfigLoadPrintsWarnings(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.Defaults()
	cfg.Warnings = []string{"keybinding attach=\"ctrl+a\" uses a reserved key"}
	reportConfigLoad(&buf, cfg, errors.New("boom"))
	out := buf.String()
	if !strings.Contains(out, "failed to load config: boom") {
		t.Errorf("load error not reported: %q", out)
	}
	if !strings.Contains(out, "reserved key") {
		t.Errorf("config warning not reported: %q", out)
	}
}

func TestReportConfigLoadSilentWhenClean(t *testing.T) {
	var buf bytes.Buffer
	reportConfigLoad(&buf, config.Defaults(), nil)
	if buf.Len() != 0 {
		t.Errorf("unexpected output: %q", buf.String())
	}
}
