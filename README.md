# security-dashboard

Turn your repository's security and quality scans into one self-contained,
shareable HTML page: evidence for your developers, and for your customers, that
the app is built with rigor.

- **Works on any stack.** Detects Go, Node, Python, PHP, Ruby, Rust, Java,
  Docker and Terraform, and proposes the right scanners for each.
- **One file to share.** `dashboard.html` has no external dependencies: open it,
  commit it, email it, or print it to PDF.
- **Tracks more than scans.** Trend over time, OWASP ASVS compliance mapping,
  a schedule of manual security tasks, and a README badge.
- **Reports to Ticket.** `bundle` produces the JSON that Ticket's admin
  **Health** dashboard accepts. See [Reporting to Ticket](#reporting-to-tickets-health-dashboard).

See [`examples/`](examples/) for real output: this tool scanned by itself.

## Install

**Prebuilt binary** (no Go needed): download the archive for your OS from the
[releases page](https://github.com/jjamieson1/security-dashboard/releases),
unpack it, and put `security-dashboard` on your `PATH`.

**With Go 1.22+:**

```sh
go install github.com/jjamieson1/security-dashboard@latest
```

Check it worked: `security-dashboard version`.

The scanners themselves (gitleaks, semgrep, govulncheck, npm audit, …) are
separate tools. A missing one is reported on the dashboard with its install
hint rather than breaking the scan.

## Quickstart

Run from the root of the repository you want to scan:

```sh
security-dashboard init        # detect the stack, write security/config.json + tasks.json
security-dashboard scan        # run every check, record the run, render the page
open security/dashboard.html   # view it (xdg-open on Linux, start on Windows)
```

Then fill in `app.team` and `app.contact` in `security/config.json`, and commit
the `security/` folder **except** raw reports. Add this to your `.gitignore`,
because raw tool output can contain sensitive detail:

```gitignore
security/runs/*/
```

Run `security-dashboard help` for every command and flag.

## Commands

| Command                            | What it does                                                         |
| ---------------------------------- | -------------------------------------------------------------------- |
| `init`                             | Detect the stack and write or merge `security/config.json`           |
| `init --check`                     | CI gate: fail on stack drift, warn on gaps. Never writes             |
| `init --dry-run`                   | Show what `init` would change                                        |
| `scan [--trigger ci]`              | Run all checks, save a run, re-render                                |
| `render`                           | Re-render `dashboard.html` from saved runs                           |
| `bundle`                           | Print a health bundle (JSON) for Ticket                              |
| `task list`                        | Show the manual-task schedule                                        |
| `task done <id> [--owner --note]`  | Mark a manual task as performed today (or `--date YYYY-MM-DD`)       |
| `version`, `help`                  |                                                                      |

All commands accept `--dir <path>` to use a folder other than `security/`.

## Use it in CI

A GitHub Actions job that gates on drift, scans, and keeps the dashboard as a
build artifact:

```yaml
security:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with: { go-version: stable }
    - run: go install github.com/jjamieson1/security-dashboard@v1.0.0  # pin a release
    - run: security-dashboard init --check
    - run: security-dashboard scan --trigger ci
    - run: security-dashboard bundle > health.json
    - uses: actions/upload-artifact@v4
      with:
        name: security-dashboard
        path: |
          security/dashboard.html
          health.json
```

Install whichever scanners your `config.json` uses in earlier steps. A
**gating** check whose tool is missing fails the scan on purpose: a gate that
never ran hasn't passed.

## What `security/` contains

```
security/
  config.json          # app metadata + the checks to run (edit this)
  tasks.json           # manual security tasks + their cadence/status (edit this)
  compliance/*.md      # optional: compliance frameworks in scope (HIPAA, GDPR, …)
  runs/<id>.json       # one recorded run: per-check status + finding counts (commit)
  runs/<id>/*.txt      # raw tool output (do NOT commit; may be sensitive)
  dashboard.html       # the generated page (commit)
  badge.svg            # posture badge for your README (commit)
```

`scan` writes run files; `render` builds the page from them. The tool is written
in Go, but everything it reads and writes is JSON, so a project in any language
gets the same dashboard.

## Generating and maintaining `config.json`

`config.json` holds three kinds of content, and only one of them can be derived
from the code:

| Content                                | Who writes it                                     |
| -------------------------------------- | ------------------------------------------------- |
| `app`, `manualDoc`, `checks[]`         | **`init`**, detected from the filesystem          |
| which compliance frameworks apply      | you, as markdown files in `security/compliance/`  |
| `standards[]`, `asvs` evidence strings | a person (or an agent) who has read the code      |

`init` never writes the third group. A generated compliance table full of
invented evidence is worse than no table, because that is exactly the section a
customer reads as a promise.

### What `init` detects

| Signal                                     | Checks proposed                                             | If missing  |
| ------------------------------------------ | ----------------------------------------------------------- | ----------- |
| `go.mod`                                   | build, vet, test, govulncheck (+ gosec)                     | drift       |
| `package.json` (per directory)             | dependency audit, run in that `dir`, npm/yarn/pnpm by lockfile | drift    |
| `pyproject.toml` / `requirements.txt`      | pip-audit (+ bandit)                                        | drift       |
| `composer.json` / `Gemfile` / `Cargo.toml` | that ecosystem's audit                                      | drift       |
| `pom.xml` / `build.gradle`                 | osv-scanner                                                 | drift       |
| any repo                                   | gitleaks                                                    | drift       |
| any repo                                   | semgrep, syft (SBOM)                                        | suggestion  |
| `Dockerfile`                               | trivy                                                       | suggestion  |
| `*.tf`                                     | checkov                                                     | suggestion  |

### Re-running `init` is safe

`init` merges. It never rewrites or deletes what it didn't write.

- Coverage is matched on what a check **does** (parser, tool, directory,
  command), not its name, so a hand-written `our-secret-sweep` check counts as
  secret scanning. You can adopt `init` in a repo with a hand-written config.
- Checks `init` adds carry `"source": "detected"`. Anything without `source` is
  yours and is left alone.
- To decline a suggestion permanently, set `"disabled": true` and a `"reason"`.
  `scan` skips it and `init` won't add it back.
- Nothing is deleted automatically. Checks that no longer match anything are
  reported.
- Unknown top-level keys are preserved.

### `init --check` (for CI)

Exits non-zero on **drift**, meaning the config no longer matches the code:

- a detected stack (say, a new frontend folder) that no check covers
- a check whose `dir` no longer exists

Exits zero with warnings for **gaps**: a tool not installed, a compliance
framework with no controls written up, ASVS rows with empty `evidence`. It never
writes, so it's safe to gate on.

## Compliance frameworks (`security/compliance/`)

Drop one markdown file per framework that applies:

```
security/compliance/hipaa.md      # first line: "# HIPAA Security Rule (45 CFR Part 164)"
security/compliance/gdpr.md
```

`init` records each in `config.json` → `complianceDocs` (id from the filename,
title from the first `#` heading), and the dashboard lists it under
**Frameworks in scope**. Until controls are written for it, it shows as
_"evidence pending"_ rather than implying it is covered.

To write the controls: add a `standards[]` entry with `"framework": "<doc id>"`,
add `asvs.requirements` rows whose `evidence` cites real files and tests, and add
any manual tasks the framework requires to `tasks.json`. Only claim
`"coverage": "automated"` when a check in `checks[]` actually proves it. Re-run
`init --check` and the "no authored controls" warning clears.

> **For AI agents:** these files are plain JSON on purpose. To add an automated
> check, edit `checks` in `config.json`; to add a manual task, edit `tasks.json`;
> then run `security-dashboard scan`. Never invent `evidence` strings.

## Adding a check

Add an object to `checks` in `config.json`. No code change needed:

```json
{
  "key": "trivy-image",
  "name": "Container image scan",
  "category": "Infrastructure",
  "tool": "trivy",
  "gating": false,
  "command": "trivy image --format json myapp:latest",
  "parser": "exit",
  "installHint": "brew install trivy"
}
```

| Field          | Meaning                                                                        |
| -------------- | ------------------------------------------------------------------------------ |
| `gating`       | a failure turns the whole scan red                                             |
| `parser`       | how output becomes a status (list below)                                       |
| `dir`          | run in a subdirectory (e.g. a frontend app)                                    |
| `probe`        | the binary to look for, when `command` is a shell pipeline                     |
| `requireEnv`   | env vars that must be set for the check to mean anything                       |
| `suppress`     | reviewed findings to dismiss: `[{ "rule", "path", "reason" }]`, all required   |
| `disabled`     | skip it on purpose; give a `reason`                                            |

Built-in parsers: `exit` (status from the exit code), `gotest`, `govulncheck`,
`gosec-json`, `semgrep-json`, `gitleaks`, `npm-audit`, `sarif` (CodeQL,
Scorecard, and any SARIF scanner), `eslint-json`, `vitest-json`. A new format is
a new `case` in `parseResult` in `checks.go`.

**When a check can't run** (tool not installed, `dir` missing, a `requireEnv`
variable unset), the dashboard records why and shows the install hint:

- a **non-gating** check is marked **skipped**;
- a **gating** check is a **failure**, because a gate that never ran hasn't
  passed.

Use `requireEnv` when a command can succeed without testing what matters, such
as a test suite that silently skips its database tests when there's no DSN:

```json
{
  "key": "tests",
  "gating": true,
  "command": "go test ./...",
  "parser": "gotest",
  "requireEnv": ["TEST_DATABASE_DSN"]
}
```

**Vendored or generated code:** exclude it from the scanner's own input (its
ignore file or command-line filter) rather than suppressing findings rule by
rule. Keep `suppress` for one specific finding, in a file you own, with a reason
the tool is wrong.

## Manual tasks

Some controls can't be automated (threat-model reviews, access reviews, restore
drills). `init` seeds `tasks.json` with a baseline set; add your own:

```json
{
  "id": "threat-model-review",
  "title": "Threat-model review",
  "category": "Design",
  "cadence": "Every release",
  "owner": "",
  "status": "pending",
  "lastRun": "",
  "docAnchor": "threat-model",
  "notes": "Re-review trust boundaries when a new integration lands."
}
```

`docAnchor` links to a heading in the playbook named by `manualDoc` in
`config.json`.

Record completion from the terminal:

```sh
security-dashboard task list
security-dashboard task done threat-model-review --owner alex --note "no findings"
```

`task done` updates `tasks.json` and re-renders. Commit the result: git history
is the audit trail for when a control was last performed and by whom.

| State                | Meaning                                                    |
| -------------------- | ---------------------------------------------------------- |
| `never run`          | no `lastRun` recorded                                      |
| `overdue`            | past `lastRun` + cadence                                   |
| `due soon`           | due within 14 days                                         |
| `current`            | inside the cadence window                                  |
| `per release`        | event-driven cadence that has been performed; no deadline  |
| `waived` / `blocked` | set `status` by hand; overrides the calendar               |

Cadences understood: `weekly` (7 days), `monthly` (30), `quarterly` (90),
`semi-annual` / `every 6 months` (182), `annual` / `yearly` (365). Anything else
(`Every release`, `Every PR`) is event-driven and gets no invented due date. Set
`intervalDays` to force a time-based deadline, or `nextDue` to pin a date.

Manual tasks don't affect the pass/fail posture or the badge: a missed review is
a scheduling problem, not a broken build. They get their own summary.

## Extras

**README badge.** Every render writes `security/badge.svg`, colored by the
latest result:

```markdown
![Security & quality](security/badge.svg)
```

**ASVS mapping.** `config.json` → `asvs` declares the OWASP ASVS level you
target and maps each control family to its evidence (`automated`, `partial`, or
`manual`). It renders as the "ASVS compliance evidence" table.

**SBOM.** The `sbom` check runs [syft](https://github.com/anchore/syft) to
produce a CycloneDX bill of materials you can hand to customers who require one.

**PDF.** The page has an **Export / Save as PDF** button with a print
stylesheet for a clean customer one-pager.

## Reporting to Ticket's Health dashboard

Ticket's admin console has a **Health** section that tracks connected
applications across security, compliance and availability. To report yours:

```sh
security-dashboard scan
security-dashboard bundle > myapp-health.json
```

The bundle contains the latest run plus the compliance evidence from
`config.json`. Optionally add an `"availability"` section from your monitoring.
Then, in Ticket's admin console, go to **Health → Add application report**
(requires the `SYSTEM_ADMIN` role) and upload the file. Uploading is manual
today; there is no API to push results from CI.

The full schema, a complete example for a non-identity-provider app, and what to
leave out are in [`docs/health-report-guide.md`](docs/health-report-guide.md).

## Further reading

- [`docs/health-report-guide.md`](docs/health-report-guide.md): the health
  bundle format and upload steps.
- [`docs/security-testing-manual.md`](docs/security-testing-manual.md): Ticket's
  own pipeline and manual playbook, as a worked example. Much of it is specific
  to Ticket being an identity provider; adapt it rather than copying it.

## Releasing (maintainers)

Tag and push; the `release` workflow builds binaries for macOS, Linux and
Windows with GoReleaser and attaches them to the GitHub release.

```sh
git tag v1.0.0 && git push origin v1.0.0
```

## License

See [LICENSE](LICENSE).
