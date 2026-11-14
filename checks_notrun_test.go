package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// APP-220: a check that could not run was "skipped" whatever its gating, and
// tally never counted skipped as a failure. A gating shellcheck that wasn't
// installed, and a gating test suite whose database tests skipped for want of
// a DSN, both read as green for weeks.
func TestAGatingCheckThatCannotRunFailsTheScan(t *testing.T) {
	const missingTool = "no-such-tool-app220"
	const unsetVar = "APP220_TEST_UNSET_VAR"
	t.Setenv(unsetVar, "")
	t.Setenv("APP220_TEST_SET_VAR", "set")

	tests := []struct {
		name        string
		def         CheckDef
		wantStatus  string
		wantSummary string // substring
		wantNotRun  bool
		wantGating  int // Totals.GatingFail for this check alone
	}{
		{
			name:       "gating, tool not installed",
			def:        CheckDef{Key: "sc", Gating: true, Command: missingTool + " -x .", Parser: "exit", InstallHint: "brew install " + missingTool},
			wantStatus: "error", wantSummary: "gating check could not run: not installed",
			wantNotRun: true, wantGating: 1,
		},
		{
			name:       "gating, probe not installed behind a shell command",
			def:        CheckDef{Key: "sc", Gating: true, Probe: missingTool, Command: "set -e; " + missingTool, Parser: "exit"},
			wantStatus: "error", wantSummary: "not installed",
			wantNotRun: true, wantGating: 1,
		},
		{
			name:       "gating, dir absent",
			def:        CheckDef{Key: "web", Gating: true, Command: "true", Dir: "no-such-dir-app220", Parser: "exit"},
			wantStatus: "error", wantSummary: "no-such-dir-app220 not present",
			wantNotRun: true, wantGating: 1,
		},
		{
			name:       "gating, required env unset",
			def:        CheckDef{Key: "tests", Gating: true, Command: "true", Parser: "exit", RequireEnv: []string{unsetVar}},
			wantStatus: "error", wantSummary: unsetVar + " not set",
			wantNotRun: true, wantGating: 1,
		},
		{
			name:       "non-gating, tool not installed, stays skipped",
			def:        CheckDef{Key: "trivy", Command: missingTool + " image x", Parser: "exit", InstallHint: "brew install " + missingTool},
			wantStatus: "skipped", wantSummary: "not installed",
			wantNotRun: true,
		},
		{
			name:       "non-gating, required env unset, stays skipped",
			def:        CheckDef{Key: "e2e", Command: "true", Parser: "exit", RequireEnv: []string{unsetVar}},
			wantStatus: "skipped", wantSummary: unsetVar + " not set",
			wantNotRun: true,
		},
		{
			name:       "gating, required env set, runs",
			def:        CheckDef{Key: "tests", Gating: true, Command: "true", Parser: "exit", RequireEnv: []string{"APP220_TEST_SET_VAR"}},
			wantStatus: "pass",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := runCheck(context.Background(), tc.def, t.TempDir())
			if res.Status != tc.wantStatus {
				t.Fatalf("status = %q (%q), want %q", res.Status, res.Summary, tc.wantStatus)
			}
			if tc.wantSummary != "" && !strings.Contains(res.Summary, tc.wantSummary) {
				t.Errorf("summary = %q, want it to contain %q", res.Summary, tc.wantSummary)
			}
			if res.NotRun != tc.wantNotRun {
				t.Errorf("notRun = %v, want %v", res.NotRun, tc.wantNotRun)
			}
			if got := tally([]CheckResult{res}).GatingFail; got != tc.wantGating {
				t.Errorf("GatingFail = %d, want %d", got, tc.wantGating)
			}
		})
	}
}

// The scan's headline must go red, and the dashboard must still say why and
// how to fix it: the install hint was only rendered for "skipped".
func TestAGatingCheckThatCannotRunShowsWhyAndTheHint(t *testing.T) {
	gating := CheckResult{CheckDef: CheckDef{Key: "sc", Name: "Shell lint", Category: "Code", Gating: true, InstallHint: "brew install shellcheck"}}
	markNotRun(&gating, "not installed")
	other := CheckResult{CheckDef: CheckDef{Key: "ok", Name: "Other", Category: "Code"}, Status: "pass"}

	run := Run{ID: "20261112T000000Z", Checks: []CheckResult{gating, other}}
	run.Totals = tally(run.Checks)
	if code, _, _ := posture(run); code != "red" {
		t.Fatalf("posture = %q, want red", code)
	}

	html := renderForTest(t, run)
	for _, want := range []string{"gating check could not run: not installed", "brew install shellcheck"} {
		if !strings.Contains(html, want) {
			t.Errorf("dashboard is missing %q", want)
		}
	}
}

// renderForTest saves run and renders the dashboard the way `scan` does, so the
// result has round-tripped through the run JSON.
func renderForTest(t *testing.T, run Run) string {
	t.Helper()
	dir := t.TempDir()
	if err := saveRun(dir, run); err != nil {
		t.Fatalf("saveRun: %v", err)
	}
	if err := render(dir, Config{}); err != nil {
		t.Fatalf("render: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "dashboard.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
