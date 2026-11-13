# Security & Quality Dashboard

A self-contained, shareable dashboard that documents every security and quality
check for this app — for developers, and as evidence for customers that we build
with rigor.

```
go run ./cmd/security-dashboard init      # scan the project, write/refresh config.json
go run ./cmd/security-dashboard init --check  # CI gate: fail on stack drift, warn on gaps
go run ./cmd/security-dashboard scan     # run all checks, record a run, re-render
go run ./cmd/security-dashboard render    # just re-render from existing runs
go run ./cmd/security-dashboard bundle > {application-name}-health.json
open security/dashboard.html              # view it
```

`dashboard.html` has **no external dependencies** — open it locally, commit it,
email it, or host it. It shows the posture at a glance, when the last
comprehensive scan ran, each check with a clickable report, a **results-over-time
trend**, an **ASVS compliance mapping**, the manual-task schedule, and the
standards we test against. Hit **Export / Save as PDF** for a customer-ready
one-pager (a print stylesheet renders it cleanly).

### README status badge

Each render also writes `security/badge.svg`, colored by the latest posture.
Embed it anywhere:

```markdown
![Security & quality](security/badge.svg)
```

### ASVS mapping

`config.json` → `asvs` declares the OWASP ASVS level the app targets and maps
each control family to its evidence (automated / partial / manual). This renders
as the "ASVS compliance evidence" table — persuasive for enterprise/gov buyers.
Update the mappings as coverage grows.

### Supply chain

- **SBOM** — the `sbom` check runs `syft` to produce a CycloneDX bill of
  materials (install `syft` to enable). Ship it to customers who require one.
- **OpenSSF Scorecard** — the `scorecard` job in `security-nightly.yml` scores the
  repo's supply-chain practices. The workflows do run; what they cannot do is gate
  a merge, because branch protection needs a paid plan on this private repo
  (APP-12). Check the job's result rather than assuming it ran and passed.

## How it works

Manifest-driven, so it's **language-agnostic** and drops into any repo:

```
security/
  config.json          # app metadata + the list of checks to run (edit this)
  tasks.json           # manual security tasks + their cadence/status (edit this)
  runs/<id>.json       # one recorded run: per-check status + finding counts (committed)
  runs/<id>/*.txt      # raw tool output for each check (gitignored — may be sensitive)
  dashboard.html       # generated evidence page (committed)
```

`scan` runs each check, writes a run manifest + raw reports; `render` builds the
HTML from all manifests. The tool is Go, but what it _reads_ is JSON — so a
Node/Python/PHP client app reuses the exact same dashboard.

## Generating and maintaining `config.json` (`init`)

`init` scans the project and writes `config.json`. It exists because the manifest
has three kinds of content, and only one of them can be derived from code:

| Content                                | Who authors it                                                                |
| -------------------------------------- | ----------------------------------------------------------------------------- |
| `app`, `manualDoc`, `checks[]`         | **`init`** — detected from the filesystem                                     |
| which compliance regimes apply         | the project's **initialization skill**, as markdown in `security/compliance/` |
| `standards[]`, `asvs` evidence strings | **the agent**, by reading the code                                            |

`init` never writes the third group. A generated compliance table full of invented
evidence is worse than no table, because that section is exactly what a customer
reads as an assertion. Instead `init` records which regimes are in scope and
reports which ones have nothing authored against them yet.

### Detection

| Signal                                     | Checks proposed                                              | Absence is   |
| ------------------------------------------ | ------------------------------------------------------------ | ------------ |
| `go.mod`                                   | build, vet, test, govulncheck (+ gosec)                      | drift        |
| `package.json` (per directory)             | dependency audit, `dir` set, npm/yarn/pnpm from the lockfile | drift        |
| `pyproject.toml` / `requirements.txt`      | pip-audit (+ bandit)                                         | drift        |
| `composer.json` / `Gemfile` / `Cargo.toml` | that ecosystem's audit                                       | drift        |
| `pom.xml` / `build.gradle`                 | osv-scanner                                                  | drift        |
| any repo                                   | gitleaks                                                     | drift        |
| any repo                                   | semgrep, syft (SBOM)                                         | a suggestion |
| `Dockerfile`                               | trivy                                                        | a suggestion |
| `*.tf`                                     | checkov                                                      | a suggestion |

### Re-running is safe

`init` merges; it never rewrites or deletes what it doesn't own.

- Coverage is matched on what a check **does** (parser, tool, directory,
  command), not on its key — so a hand-written `our-secret-sweep` check counts as
  secret scanning and `init` stays quiet. This is what lets `init` be introduced
  to a repo whose manifest was written by hand.
- Checks `init` adds carry `"source": "detected"`. Anything without a `source` is
  hand-authored by definition and is left alone.
- To decline a suggestion permanently, set `"disabled": true` with a `"reason"`.
  `scan` skips it and `init` treats the signal as covered, so it is not
  resurrected.
- Nothing is ever deleted automatically. Orphans are reported.
- Unknown top-level keys survive the rewrite, so a newer schema or a per-app
  extension is not silently dropped.

### `init --check`

It is **gating** in `scripts/security.sh --strict` (the "Security-manifest drift" section), so
a subproject added without a check fails the run rather than passing quietly. That gap was
real: `mcp/` shipped with no dependency audit at all, and nothing noticed until `init --check`
was run by hand. `init --check` never rewrites `config.json`, which is what makes it safe to
gate on.

Exits non-zero on **drift** — the manifest no longer matches the stack:

- a detected stack signal that no check covers (a new SPA folder with no audit)
- a check whose `dir` no longer exists, so it could only ever error

Exits zero, printing warnings, for **gaps** — a tool not installed, a framework
in scope with no controls authored, ASVS rows with empty `evidence`. Adopting a
new framework must not red-build every app until every control is written up.
Wired into `.github/workflows/ci.yml` in the `go` job — which does not execute
here, so run it yourself alongside `scripts/security.sh --strict` (APP-12).

## Compliance goals (`security/compliance/`)

The project initialization skill drops framework docs here:

```
security/compliance/hipaa.md      # "# HIPAA Security Rule (45 CFR Part 164)"
security/compliance/gdpr.md
```

`init` records each one in `config.json` → `complianceDocs` (id from the
filename, title from the first `#` heading) and the dashboard renders a
**Frameworks in scope** section. A regime with no authored controls shows as
_"evidence pending"_ rather than being implied to be covered.

> **For Claude:** after `init`, read each doc in `security/compliance/` and author
> the mapping it implies — add a `standards[]` entry with `"framework": "<doc id>"`
> to link it, extend `asvs.requirements` with real `evidence` strings citing files
> and tests, and add any manual tasks the regime requires to `tasks.json`. Only
> claim `coverage: "automated"` where a check in `checks[]` actually proves it.
> Then re-run `init --check` — the "no authored controls" warning should clear.

## Adding a check (an automated scan)

Add an object to `checks` in `config.json`. No code change:

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

Built-in `parser` values: `exit` (status from exit code), `gotest`,
`govulncheck`, `gosec-json`, `semgrep-json`, `gitleaks`, `npm-audit`. Use
`"dir"` to run in a subdirectory (e.g. a SPA).

A check that **cannot run** (its tool isn't installed, its `dir` is absent, or a
variable in `requireEnv` is unset) is recorded with the reason and its install
hint. What happens next depends on `gating`:

- **non-gating**: **not run** (`skipped`), so the dashboard shows what's
  available to enable;
- **gating**: a **failure**: "gating check could not run: …". A gate that never
  ran hasn't passed, so it turns the scan red (APP-220).

Use `requireEnv` when the command can succeed without having tested what
matters, for example a suite whose database tests `t.Skip` without a DSN and
still exit 0:

```json
{
  "key": "tests",
  "gating": true,
  "command": "go test ./...",
  "parser": "gotest",
  "requireEnv": ["BD_TEST_MYSQL_ROOT_DSN"]
}
```

Need a new output format? Add a `case` to `parseResult` in
`cmd/security-dashboard/checks.go`.

## Excluding vendored code from a scanner

Two directories hold JavaScript we did not write and cannot fix:
`internal/httpapi/mcpbundle/` (esbuild output containing the MCP SDK, committed
so `go build` never needs npm) and `design/design_handoff_user_portal_redesign/`
(a design reference the app must never import from).

They are excluded from the CodeQL **database**, via `LGTM_INDEX_FILTERS` on the
`codeql-js` command, rather than suppressed rule by rule. Suppression is the
wrong instrument here: any JavaScript query can trip on 1.3MB of vendored code,
so per-rule entries become whack-a-mole, and each one also blinds us to a
genuine finding in any _other_ file that happens to share that rule. CodeQL
reads neither `.gitignore` nor `.semgrepignore`, so the filter has to be stated
on the command.

Keep `suppress` for what it is good at: a specific finding, in a specific file
we own, with a reason explaining why the tool is wrong.

## Adding a manual task

Add an object to `tasks` in `tasks.json`:

```json
{
  "id": "threat-model-review",
  "title": "Threat-model review",
  "category": "Design",
  "cadence": "Every release",
  "owner": "",
  "status": "pending",
  "lastRun": "",
  "docAnchor": "a-broken-access-control--multi-tenancy--every-release",
  "notes": "Re-review trust boundaries when a new integration lands."
}
```

`docAnchor` links the task to a heading in the manual playbook
(`docs/security-testing-manual.md`). Re-render to update the page.

## Tracking a manual task

Cadence is enforced, not decorative. `lastRun` + `cadence` yield a due date and a
freshness state, so an overdue review is as visible as a failing check:

```
go run ./cmd/security-dashboard task list                  # the schedule, in the terminal
go run ./cmd/security-dashboard task done <id>     --owner jamie --note "no findings" [--date 2026-08-29]  # stamp it as performed
```

`task done` writes `lastRun` (and `owner`/`lastNote`) into `tasks.json` and
re-renders. **Completion is a commit** — git history, not memory, is the audit
trail for when a manual control was last exercised and by whom.

| State                | Meaning                                                   |
| -------------------- | --------------------------------------------------------- |
| `never run`          | no `lastRun` recorded — a control never performed         |
| `overdue`            | past `lastRun + cadence`                                  |
| `due soon`           | due within 14 days                                        |
| `current`            | inside the cadence window                                 |
| `per release`        | event-driven cadence, on record — no deadline is invented |
| `waived` / `blocked` | set `status` by hand; overrides the calendar              |

Cadence text is parsed for `weekly` (7d), `monthly` (30d), `quarterly` (90d),
`semi-annual` / `every 6 months` (182d), `annual` / `yearly` (365d). Anything
else — `Every release`, `Every PR` — is **event-driven**: real, but not a clock,
so no due date is fabricated from a guess about release frequency. Set
`intervalDays` on a task to give it a time-based deadline anyway, or `nextDue`
to pin one agreed date.

The manual-task section is deliberately excluded from the gating posture and the
README badge: a process lapse is a scheduling problem, not a build failure. It
gets its own summary pill instead.

> **For Claude:** to "add a task to an application scan," edit `config.json`
> (automated check) or `tasks.json` (manual task), then run
> `go run ./cmd/security-dashboard scan`. Both files are plain JSON by design so
> this is a safe, mechanical edit.

## Skills

Two skills wrap this tooling:

- **`build-security-pipeline`** — stand the pipeline up in a client
  application: copy the tool, run `init`, author the compliance mapping, wire CI,
  and generate a playbook for _that_ app.
- **`build-security-dashboard`** — implement a dashboard like this one from
  scratch, in another language. The design and the failure modes, not the Go.

## Reusing this for a client application

To give a client app the same rigor:

1. Copy `cmd/security-dashboard/` into that repo (no need to copy `security/` —
   `init` creates it).
2. Run `go run ./cmd/security-dashboard init`. It detects the stack, writes
   `config.json` and seeds `tasks.json` with the baseline manual tasks. Review
   the result and fill in `app.team` / `app.contact`.
3. Add the `security-*` jobs from `.github/workflows/` (or just the `dashboard`
   job) to that repo's CI.
4. `go run ./cmd/security-dashboard scan` and commit `security/dashboard.html`.

The manifest format is the contract; everything else is per-app configuration.

## Reporting a client app to Ticket's Health dashboard

Ticket's admin console has a **Health** section that tracks connected applications
across security, compliance and availability. To surface a client app there:

```sh
go run ./cmd/security-dashboard bundle > myapp-health.json
```

`bundle` emits a **health bundle** — the latest security run plus the compliance
evidence from `config.json`:

```json
{ "app": {...}, "security": { ...run... }, "compliance": { "asvsLevel": "...", "conformance": [...] } }
```

Add an `"availability"` section (`{ "status": "healthy", "latencyMs": 42, "detail": "..." }`)
from your monitoring if you have it. Then in the admin, **Health → Add
application report** (SYSTEM_ADMIN) and upload the file. A bare run JSON is also
accepted (security only).

**Full integrator guide (schema + a client-app example that omits identity-provider
specifics):** [`docs/health-report-guide.md`](../docs/health-report-guide.md).

See also: `docs/security-testing-manual.md` (the full pipeline + manual playbook).
