package main

import "testing"

// APP-221: the govulncheck parser used to read only the output, so a run that
// failed without printing findings (packages that don't load, an unreachable
// vulnerability database, a wrapper that exits 1 when the tool is missing)
// came out "pass — no known vulnerable dependencies called". The gate was green
// having checked nothing.
func TestGovulncheckParserNeedsACleanExitToPass(t *testing.T) {
	const clean = "=== Symbol Results ===\n\nNo vulnerabilities found.\n\n" +
		"Your code is affected by 0 vulnerabilities.\n" +
		"This scan also found 1 vulnerability in packages you import and 2\n" +
		"vulnerabilities in modules you require, but your code doesn't appear to call\nthese vulnerabilities.\n"
	const affected = "=== Symbol Results ===\n\nVulnerability #1: GO-2025-0001\n    ...\n" +
		"Vulnerability #2: GO-2025-0002\n    ...\n\nYour code is affected by 2 vulnerabilities from 1 module.\n"

	tests := []struct {
		name, output string
		exit         int
		wantStatus   string
		wantFindings int
	}{
		{name: "exit 0 clean passes", output: clean, exit: 0, wantStatus: "pass"},
		{name: "exit 3 with findings fails", output: affected, exit: 3, wantStatus: "fail", wantFindings: 2},
		{
			name:   "entries without a summary still count",
			output: "Vulnerability #1: GO-2025-0001\n", exit: 3,
			wantStatus: "fail", wantFindings: 1,
		},
		{
			name: "non-zero exit with only an error is an error",
			output: "govulncheck: loading packages: There are errors with the provided package patterns:\n" +
				"/src/app/main.go:4:2: undefined: foo\n",
			exit: 1, wantStatus: "error",
		},
		{name: "unreachable vuln DB is an error", output: "govulncheck: fetching vulnerabilities: dial tcp: lookup vuln.go.dev: no such host\n", exit: 1, wantStatus: "error"},
		{name: "empty output with non-zero exit is an error", output: "", exit: 1, wantStatus: "error"},
		{
			// A wrapper (`command -v govulncheck || exit 1; ...`) failing.
			name: "missing tool behind a wrapper is an error", output: "", exit: 127, wantStatus: "error",
		},
		{
			// A clean-looking summary is not enough if the process still failed.
			name: "clean summary with an unexpected exit is an error", output: clean, exit: 2, wantStatus: "error",
		},
		{
			// Output in a format we no longer recognise must not read as clean.
			name: "unrecognised output on exit 0 is an error", output: "Scanning your code...\nDone.\n", exit: 0, wantStatus: "error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := &CheckResult{CheckDef: CheckDef{Parser: "govulncheck"}}
			parseResult(res, tc.output, tc.output, tc.exit)
			if res.Status != tc.wantStatus {
				t.Errorf("status = %q (%q), want %q", res.Status, res.Summary, tc.wantStatus)
			}
			if res.Findings != tc.wantFindings {
				t.Errorf("findings = %d, want %d", res.Findings, tc.wantFindings)
			}
		})
	}
}
