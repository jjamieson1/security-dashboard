# Creating a Health Report for a Connected Application

Connected applications (permitting, facility booking, licensing, …) report their
own **security, compliance and availability** to the Ticket admin **Health**
dashboard by uploading a **health bundle** — a single JSON file.

This guide shows how to produce one for _your_ application.

> **Report only what your app actually is.** Ticket is an identity provider, so its
> report includes things like OIDC/SAML conformance and federation controls. A
> permitting or booking app is **not** an identity provider — **omit** those.
> Report your app's real surface: its endpoints, dependencies, and data handling.
> See ["What to exclude"](#what-to-exclude) at the end.

---

## The bundle format

One JSON file with up to three sections. **`security` is required**; `compliance`
and `availability` are optional (add them as you have the data).

```json
{
  "app": { "name": "Permitting Portal", "description": "..." },
  "security": { "...": "a scan run (see below)" },
  "compliance": { "...": "optional" },
  "availability": { "...": "optional" }
}
```

The **application name** is entered in the upload form, so `app` is optional
metadata. A bare security run (a file whose top level is the run itself, i.e. it
has a `checks` array) is also accepted — security only.

---

## Two ways to produce the bundle

### A. With the `security-dashboard` tool (easiest)

Install the tool (see the [README](../README.md#install)), let `init` generate a
`security/config.json` for your stack, review the checks and compliance it
proposes, then:

```sh
security-dashboard init                       # first time only
security-dashboard scan                       # run your configured checks
security-dashboard bundle > myapp-health.json
```

[`examples/health-bundle.json`](../examples/health-bundle.json) shows real output.

The tool emits the correct shape automatically (including the awkward `totals`
key casing noted below).

### B. Hand-authored from your own CI

If your app isn't Go, generate the JSON from your existing pipeline — map your
build/test/lint/scan steps to `checks`. Use the schema reference below.

---

## Section reference

### `security` (required)

A scan **run**:

| Field       | Type   | Notes                                                 |
| ----------- | ------ | ----------------------------------------------------- |
| `id`        | string | Any unique run id (e.g. a timestamp)                  |
| `startedAt` | string | RFC 3339 (`2026-08-25T14:00:00Z`)                     |
| `gitSha`    | string | Commit under test                                     |
| `trigger`   | string | `ci` / `manual` / `nightly`                           |
| `checks`    | array  | **≥ 1 required** — see below                          |
| `totals`    | object | `{ "Pass", "Fail", "Warn", "Skipped", "GatingFail" }` |

> ⚠️ **`totals` keys are capitalized** — `Pass`, `Fail`, `Warn`, `Skipped`,
> `GatingFail` (not lowercase). The `bundle` tool handles this for you.

Each **check**:

| Field      | Type   | Notes                                                      |
| ---------- | ------ | ---------------------------------------------------------- |
| `name`     | string | e.g. "Dependency audit"                                    |
| `category` | string | free-form grouping, e.g. `Dependencies`, `SAST`, `Quality` |
| `tool`     | string | e.g. `npm audit`, `trivy`                                  |
| `gating`   | bool   | whether a failure should block a release                   |
| `status`   | string | `pass` \| `fail` \| `warn` \| `skipped`                    |
| `summary`  | string | one line, e.g. "0 critical, 0 high"                        |
| `findings` | int    | count                                                      |

Typical checks for a web/API app: **Build**, **Unit/integration tests**,
**Dependency audit**, **SAST**, **Secret scan**, **Access-control / IDOR tests**,
**Container/IaC scan**. _Not_ OIDC/SAML conformance or federation-key checks.

### `compliance` (optional)

| Field          | Type   | Notes                                                             |
| -------------- | ------ | ----------------------------------------------------------------- |
| `asvsLevel`    | string | ASVS level you target: `L1` / `L2` / `L3`                         |
| `note`         | string | short context                                                     |
| `standards`    | array  | `[{ "name", "note" }]`                                            |
| `requirements` | array  | `[{ "id", "name", "coverage", "evidence" }]`                      |
| `conformance`  | array  | `[{ "name", "suite", "status", "summary", "lastRun", "report" }]` |

- `requirements[].coverage`: `automated` \| `partial` \| `manual`
- `conformance[].status`: `passed` \| `pending` \| `issues`

Include the control families your app **actually implements** (authentication,
session management, access control, cryptography, logging, configuration). Add a
`conformance` entry only for formal conformance _your_ app runs — e.g. **WCAG**
accessibility, or **PCI-DSS** if you take card payments. **Omit** OIDC/SAML
conformance and federation control families (ASVS V11) unless you are yourself an
identity/federation provider.

### `availability` (optional)

| Field       | Type   | Notes                             |
| ----------- | ------ | --------------------------------- |
| `status`    | string | `healthy` \| `degraded` \| `down` |
| `checkedAt` | string | RFC 3339                          |
| `latencyMs` | int    | last probe latency                |
| `detail`    | string | e.g. "uptime 99.95% (30d)"        |

Populate from your monitoring/uptime system.

---

## Full example — "Permitting Portal" (no OIDC/SAML)

A complete, client-appropriate bundle. Note it reports app security, generic
web-app compliance, and availability — and contains **no** identity-provider
items.

```json
{
  "app": {
    "name": "Permitting Portal",
    "description": "Online building-permit applications"
  },
  "security": {
    "id": "20260825T140000Z",
    "startedAt": "2026-08-25T14:00:00Z",
    "gitSha": "a1b2c3d",
    "trigger": "ci",
    "checks": [
      {
        "name": "Build",
        "category": "Quality",
        "tool": "node",
        "gating": true,
        "status": "pass",
        "summary": "compiled",
        "findings": 0
      },
      {
        "name": "Unit tests",
        "category": "Quality",
        "tool": "vitest",
        "gating": true,
        "status": "pass",
        "summary": "412 passed",
        "findings": 0
      },
      {
        "name": "Dependency audit",
        "category": "Dependencies",
        "tool": "npm audit",
        "gating": true,
        "status": "pass",
        "summary": "0 critical, 0 high",
        "findings": 0
      },
      {
        "name": "SAST",
        "category": "SAST",
        "tool": "semgrep",
        "gating": false,
        "status": "warn",
        "summary": "3 low findings",
        "findings": 3
      },
      {
        "name": "Secret scan",
        "category": "Secrets",
        "tool": "gitleaks",
        "gating": true,
        "status": "pass",
        "summary": "no secrets",
        "findings": 0
      },
      {
        "name": "Access-control tests",
        "category": "Access control",
        "tool": "jest",
        "gating": true,
        "status": "pass",
        "summary": "IDOR/role matrix",
        "findings": 0
      }
    ],
    "totals": { "Pass": 5, "Fail": 0, "Warn": 1, "Skipped": 0, "GatingFail": 0 }
  },
  "compliance": {
    "asvsLevel": "L2",
    "note": "OWASP ASVS L2 for an application handling personal data.",
    "standards": [
      { "name": "OWASP Top 10", "note": "covered by SAST + tests" }
    ],
    "requirements": [
      {
        "id": "V2 Authentication",
        "name": "Login via Ticket SSO",
        "coverage": "automated",
        "evidence": "auth integration tests"
      },
      {
        "id": "V3 Session Management",
        "name": "Secure cookies + CSRF",
        "coverage": "automated",
        "evidence": "session tests"
      },
      {
        "id": "V4 Access Control",
        "name": "Applicant/officer authz",
        "coverage": "automated",
        "evidence": "access-control tests"
      },
      {
        "id": "V7 Errors & Logging",
        "name": "Structured audit logging",
        "coverage": "partial",
        "evidence": "log review"
      },
      {
        "id": "V14 Configuration",
        "name": "No committed secrets; dep hygiene",
        "coverage": "automated",
        "evidence": "gitleaks + npm audit"
      }
    ],
    "conformance": [
      {
        "name": "WCAG 2.1 AA (accessibility)",
        "suite": "axe + manual audit",
        "status": "passed",
        "summary": "0 critical issues",
        "lastRun": "2026-08-20",
        "report": "reports/wcag-2026-08-20.pdf"
      }
    ]
  },
  "availability": {
    "status": "healthy",
    "checkedAt": "2026-08-25T14:05:00Z",
    "latencyMs": 38,
    "detail": "uptime 99.95% (30d)"
  }
}
```

---

## Uploading

In the Ticket admin console:

1. **Health** → **Add application report** (requires the `SYSTEM_ADMIN` role)
2. Enter the application **name** (and optional description)
3. Choose your bundle **JSON file**
4. **Upload report**

The application appears in the Health list with its Security / Compliance /
Availability chips, and its full report opens in the main window. Upload a new
bundle any time to update it (the latest per application is shown).

---

## What to exclude

These belong to the **Ticket platform** because it is an identity provider — a normal
connected application should **not** report them:

- **OIDC** Basic OP / **SAML** conformance results (you consume SSO; you don't run an IdP)
- **Federation** control families (ASVS **V11**), signing-key / JWKS management checks
- Consent-policy / attribute-release / broker checks
- Anything describing your app _as_ an identity or federation provider

Report your application's own surface instead: its build/test quality, its
dependencies, its SAST/secret hygiene, its access-control tests, the web-app
control families it implements, and any conformance _it_ is subject to
(accessibility, PCI, sector-specific).
