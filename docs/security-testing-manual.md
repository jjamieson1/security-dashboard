# Security testing — pipeline & manual playbook

> 📊 **Dashboard:** run `go run ./cmd/security-dashboard scan` and open
> `security/dashboard.html` for a one-page, shareable view of all checks, the
> last scan time, clickable reports, and this playbook's task schedule. See
> `security/README.md`.

How DevPro Trust Platform (DTP) is tested for vulnerabilities. Two halves:

1. **Automated checks** — run by the developer before every PR, with
   `scripts/security.sh --strict`. Catches known-CVE deps, insecure code
   patterns, secrets, broken authz, and spec drift.
2. **Manual playbook** — the classes automation *cannot* find: business logic,
   deep protocol attacks (SAML/OIDC/JWT), and exploratory testing. Scheduled and
   checklist-driven.

> **The GitHub workflows run, but they cannot gate a merge.** Branch protection
> needs a paid plan on this private repo, so a red check blocks nothing — three
> PRs merged with two jobs failing before anyone noticed. CI is a useful second
> opinion (a clean checkout, a pinned toolchain, a Linux runner catch things a
> developer's machine does not), so read it before merging — but **the control is
> still the local script, run by a person**. That is a weaker guarantee than a
> pipeline that can stop you, and this page says so rather than implying a gate
> that is not there (APP-12).
>
> Keep the workflows' Go pin in step with the toolchain developers run. The jobs
> `go install` golangci-lint and govulncheck, both of which require a recent Go,
> and `GOTOOLCHAIN=local` means a stale pin fails at install rather than
> auto-upgrading — which is precisely how those two jobs sat red unnoticed while
> every local run was green.

> Scope: only ever run active/intrusive scans against **staging or local** — never
> production, never infrastructure you don't own.

---

## 1. Automated checks

### Before every PR — `scripts/security.sh --strict`

This is the control. Run it locally; it exits non-zero on the first gating
finding. `.github/workflows/ci.yml` mirrors it and would run the same checks if
Actions were enabled.

| Check | Tool | Finds | Gating? |
|---|---|---|---|
| Go dependencies | **govulncheck** | CVEs in Go deps you actually call | ✅ gates |
| Go SAST | **gosec** | SQL building, weak crypto/RNG, cmd injection | report-only¹ |
| Cross-lang SAST | **Semgrep** (`owasp-top-ten`, `golang`, `typescript`, `react`) | OWASP Top 10 patterns | report-only¹ |
| Secrets | **gitleaks** | committed keys/tokens/credentials | report-only¹ |
| SPA deps | **npm audit** (`web`, `web-admin`) | CVEs in npm deps | report-only¹ |
| **Authz / IDOR** | **Go tests** (`authz_matrix_test.go`, `security_test.go`) | broken object/function-level authz | ✅ gates |
| Docs freshness | `gen-openapi --check` | API spec drift | ✅ gates |

¹ *Report-only* = runs and prints findings but does not fail the run. Triage,
then promote to gating.

Two more gate today that the table above predates: **golangci-lint**
(staticcheck/errcheck — added after two structural guards shipped with lint
failures nothing caught) and **accessibility** (`scripts/a11y.sh`; see
`docs/accessibility-testing.md`).

### The wider scan — `go run ./cmd/security-dashboard scan`

Broader than the pre-PR gate and slower: it adds **CodeQL** (Go + TypeScript),
Semgrep, an SBOM, both SPA suites, and the JWT/SAML attack tests, and renders
`security/dashboard.html`. Run it when the posture matters — before a release, or
after security-relevant work — not on every commit.

### The nightly workflow — not running, and mostly not needed

`.github/workflows/security-nightly.yml` exists and does not execute. Of its
three jobs, only one represents a capability that is actually lost:

| Check | Tool | Status |
|---|---|---|
| Semantic SAST | **CodeQL** | **Covered locally** — `codeql-go` and `codeql-js` run in the dashboard scan |
| Deep SCA | **osv-scanner** | **Largely redundant** — govulncheck covers Go, npm audit covers both SPAs. osv adds cross-ecosystem breadth and the Trust Node SDK's `go.sum` |
| **DAST** (live app) | **OWASP ZAP** API scan | **NOT PERFORMED.** No local equivalent exists |

**Dynamic testing now runs locally** — `scripts/dast.sh` (APP-25). It builds the
API, starts a throwaway instance on a disposable SQLite database, mints a
read-only scanner token, runs the ZAP API scan from a container against the
committed spec, and tears everything down. Registered as a non-gating `dast`
check in `security/config.json`, so `security-dashboard scan` picks it up. It is
**not** in `scripts/security.sh`: that gate has to stay fast, and a slow check
gets skipped.

Findings are triaged in `security/zap-rules.tsv`, each ignore carrying its reason.
Steady state today is 0 fail / 1 warn / 8 ignored / 109 pass — the warn is the
payment stub's checkout form, left visible on purpose.

**What it cannot see**, and this is the important half: it scans the Go API directly, with no
Apache in front, so the CSP, the reverse-proxy path mapping and TLS are all
invisible to it. Both of this project's deployment defects lived exactly there —
a CSP correct in the repo that never reached the host, and a docs page whose URLs
were right in the binary and wrong behind the proxy.

That half is `scripts/check-edge-headers.sh`, which runs against a **deployed**
edge after every deploy. It now compares the live CSP against the vhost in git
(a stale config is a failure, not a passing "header present"), and checks that
`/api` proxies rather than falling through to the SPA, that both SPAs serve, and
that the documentation is reachable — by **content type, not status**, because a
history fallback answers 200.

Together: `dast.sh` covers the application, `check-edge-headers.sh` covers the
composition. Neither substitutes for the other.

**If CI is ever restored**, the ZAP job needs:
- Variable `STAGING_BASE_URL` — e.g. `https://staging.example.com/dtp`
- Secret `SCANNER_PAT` — a `dtp_pat_...` token for a **low-privilege, disposable**
  scanner test-user on staging. (The PAT bearer scheme is CSRF-exempt and needs no
  login flow — ideal for automated authenticated scanning.)

Never point it at muni-demo: an active scan submits forms and follows destructive
links, and that host holds the demo data.

### Running the checks

```sh
scripts/security.sh            # everything installed, report findings
scripts/security.sh --strict   # fail fast on the first gating finding
```

Install the local tools once:
```sh
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2   # pinned, as in ci.yml
brew install gitleaks semgrep        # or pipx install semgrep
```

**After upgrading Go, reinstall golangci-lint.** It embeds the toolchain it was
built with, and a binary built against an older Go panics while type-checking the
newer standard library:

```
panic: file requires newer Go version go1.27 (application built with go1.26)
```

That reads like a repository problem and is not one — `go test ./...` passes
throughout. `go install` rebuilds it against the Go on your PATH, which is why
the command above pins a version rather than a prebuilt binary.

### Pre-commit (optional but recommended)

Add a `.git/hooks/pre-commit` (or use the `pre-commit` framework):
```sh
#!/bin/sh
gitleaks git --staged --no-banner --redact || exit 1
go run ./cmd/gen-openapi --check || { echo "OpenAPI stale — run gen-openapi"; exit 1; }
```

### When you touch security-relevant code

- New/changed **route or request/response struct** → update `openapi_annotations.go`,
  run `gen-openapi`, and add the route to the tables in `authz_matrix_test.go` if
  it's identity-scoped or role-gated.
- New **operator permission** → extend `roleGatedRoutes` so the gate is tested.
- New **screen or shared UI component** → add it to the `screens` table in
  `src/test/a11y.test.tsx` and run `scripts/a11y.sh`. WCAG 2.1 AA is a hard requirement and
  is now gated; `docs/accessibility-testing.md` records what the automation cannot see, so
  the claim stays scoped to what is actually checked (APP-18).
- New **ownership- or tenant-scoped service method** → the cross-owner sweeps
  (`internal/identity/cross_owner_test.go`, `internal/rbac/cross_tenant_sweep_test.go`) fail until it
  has a case. They drive the service directly rather than through the router, because the ownership
  guard is on the route and the data has its own clause: a background job, a partner endpoint or an
  admin tool calling the service does not inherit the route's protection (APP-19).
- New handler **binding a domain model from the request body** → `body_binding_test.go` fails until
  the binding is narrowed or justified. Four separate tickets (APP-2, APP-4, APP-17, APP-20) were
  the same defect: a model carrying a field that decides whose record it is, bound straight from
  the body. Worth noting how the APP-17 instance was found — the cross-tenant tests missed it,
  because they put the foreign application in the query string where the middleware refuses it.
  Testing the obvious path of an attack is not the same as testing the attack.
- Run Claude Code's **`/security-review`** on the branch before merging.

---

### Giving a client application the same pipeline

This document is **DTP's**, and roughly a fifth of it is specific to DTP being an
identity provider — §B in particular assumes you *are* the IdP, where a client
app is a relying party and needs the opposite checks. Do not hand this file to an
integrator.

Use the **`build-security-pipeline`** skill instead. It reuses
`cmd/security-dashboard/`, adapts these workflows, authors the compliance mapping
against code that actually exists, and generates a playbook derived for that
application rather than copied from this one.

## 2. Manual playbook (what automation can't find)

Automated scanners are blind to *intent*. These items need a human (or a
protocol-specific tool) and a schedule. Track each release/quarter.

### A. Broken access control & multi-tenancy — **every release**

The automated `authz_matrix_test.go` covers the routes listed in it; it is not
exhaustive. Manually verify:

- [ ] **IDOR sweep** — for each new identity-scoped resource, confirm user A
      cannot read/modify user B's object by swapping the id. Add it to
      `identityScopedGETs` so it's covered going forward.
- [ ] **Function-level authz** — every new management route is gated by the
      correct `adminPolicy` role; add it to `roleGatedRoutes`.
      `adminAuthz` **fails closed**: an /api route with no rule is denied, and
      `TestEveryAPIRouteHasAnAuthzDecision` fails the build on any route that is
      in neither `adminPolicy` nor `publicRoutes`. It used to fall through to the
      handler, which is how `/rbac` shipped with 17 unauthenticated-in-practice
      routes. `open: true` is a decision and must say which other control covers
      the route; silence is not.
- [ ] **Tenant isolation** — across the Site → Org → App hierarchy, an operator
      scoped to Org X cannot read/write Org Y's applications, policies, forms, or
      data bundles. (No automated coverage today — highest-value manual area.)
- [ ] **PAT scope** — a read-only PAT cannot perform mutations; a PAT cannot
      exceed its user's permissions.
- [ ] **Consent enforcement** — federated attribute release honors the consent
      policy; the break-glass/emergency override is audited every time.

### B. Federation protocol attacks — **quarterly + on federation changes**

Generic scanners barely touch these; DTP *is* an IdP, so this is core.

**OIDC / OAuth2**
- [ ] Run the **OpenID Foundation Conformance Suite** against staging
      (`/oidc`). Track results next to `docs/oidc-conformance-*.md`.
- [ ] Verify PKCE is enforced (regression guard for the earlier fix); `state`/
      `nonce` are validated; `redirect_uri` is exact-match; auth codes are
      single-use and short-lived.
- [ ] `token`/`introspect`/`revoke` authenticate the client, not a session.

**SAML** — *the mechanical mutation set is now automated*

Gating check `saml-xsw-xxe` runs on every scan:

| Suite | Covers |
|---|---|
| `internal/saml/verify_xsw_test.go` | 9 XSW variants — wrap, reference-move, hidden in `Extensions`, hidden in the Signature's `Object`, duplicate-ID collision, forged sibling, repointed `Reference URI`, empty `Reference URI` (whole-document), signature stripped. Each drives a genuinely crewjam-signed AuthnRequest, so a rejection is the wrapping being caught and not an incidental parse failure. |
| `internal/saml/verify_xxe_test.go` | XXE across **every** XML entry point DTP has — `encoding/xml` (`messageIssuer`, `parseLogoutRequest`, `messageIDInstant`), `etree`, and crewjam's `samlsp.ParseMetadata`. `file://` entities, external DTD fetch (asserts zero hits on a canary server), parameter entities, billion laughs, and a DEFLATE decompression bomb on the redirect binding. Also pins comment-truncation (CVE-2017-11427 class). |

This is the evidence behind the `#nosec G709` annotations on those call sites.

The manual run is now **exploratory only** — novel **SAML Raider** / **WS-Attacker**
mutations that the fixed table above does not express. Annually, or when the SAML
code changes shape.

- [ ] Assertion replay, audience/recipient/`NotOnOrAfter` validation, and
      signature-required-on-response all hold.

**JWT** — *fully automated, no manual run*

DTP accepts externally-signed JWTs in exactly two places, and both carry the full
attack table (gating checks `jwt-attacks`, `jwt-attacks-sdk`):

| Suite | Verifier |
|---|---|
| `internal/partner/assertion_jwt_attacks_test.go` | `partner.AuthenticateAssertion` — the `private_key_jwt` client assertion (RFC 7523) |
| `sdk/trustnode/auth_jwt_attacks_test.go` | the Trust Node SDK's inbound bearer verifier — shipped to every integrator, so a hole here is a hole in every connected node |

Both cover: `alg:none` (and `None`/`NONE`/absent), RSA→HMAC key confusion (PKIX
DER, PKCS#1 DER and raw modulus as the HMAC secret), algorithms outside the
allowlist, unknown/blank `kid`, cross-client key confusion, payload tampering
after signing, stripped signatures, and unbounded lifetimes.

`jwt_tool` is only worth reaching for against a *black-box* issuer. These
verifiers are ours, so the table belongs in CI.

> **Known gap — `jti` replay.** Neither verifier tracks `jti`, so an assertion
> captured in transit can be replayed until it expires. The lifetime caps
> (10 min for client assertions, 5 min for SDK tokens) bound the window; single-use
> `jti` tracking is tracked under §D (business logic — token reuse).

### C. Dynamic / exploratory with Burp Suite — **quarterly**

ZAP is not running (APP-25); use **Burp Suite** for the hands-on depth ZAP
misses:
- [ ] Proxy a full authenticated session through Burp; exercise auth, payments,
      and consent flows manually.
- [ ] Active-scan the flows Burp records; triage findings.
- [ ] Import `docs/openapi.json` into Burp so its site map is complete.
- [ ] Session management: fixation, logout invalidation, back-channel logout
      fan-out, cookie flags (`HttpOnly`/`Secure`/`SameSite`), CSRF on cookie-auth
      mutations.

### D. Business logic — **every release**

No tool understands your rules. Walk through:
- [ ] Payment/invoice flows — negative amounts, currency/rounding, replayed
      gateway callbacks, double-submit, state-machine skips.
- [ ] Registration/activation and password-reset flows — token reuse, expiry,
      enumeration, rate-limit bypass.
- [ ] Emergency-access break-glass — approval required, time-boxed, fully audited.

### Validating the CSP — **it now enforces**

The portal ships an **enforcing** Content-Security-Policy
(`deploy/apache/dtp.conf.tmpl`). It spent its first iteration report-only so the
policy could be proven against real flows before it was allowed to break them;
that period is over, and a violation is now a broken flow rather than a console
line. A wrong CSP does not degrade this portal, it breaks it — a blank SPA, or
federated login that silently stops working.

That makes this checklist a **release gate, not a one-off**: run it against
staging after any change to the SPA bundle, the SAML post-binding pages, or a
third-party script (reCAPTCHA, fonts). To roll back, rename both headers to
`Content-Security-Policy-Report-Only` and reload Apache — both or neither, since
leaving only the `/saml` block enforcing breaks federated logout alone, which is
the flow least likely to be noticed.

Two policies are set, deliberately. The SPA gets `form-action 'self'`; the SAML
HTTP-POST binding pages under `/dtp/saml` cannot, because they auto-submit to the
**relying party's** ACS/SLO URL, which is by definition another origin. A single
blanket policy with `form-action 'self'` would report (and later block) every
federated login.

**Run these against staging with devtools open, and treat any violation as a
finding:**

- [ ] Load the portal cold (hard refresh). Confirms `script-src`/`style-src` for
      the Vite bundle, the Google Fonts `@import` in `web/src/index.css`, and
      Tailwind's `data:` SVG background.
- [ ] Register a new user — this is the only page that loads **reCAPTCHA**
      (`web/src/components/Recaptcha.tsx`), so it is the only one exercising
      `script-src https://www.google.com`, `frame-src` and `connect-src`.
- [ ] Log in, then **download an export** (`/identities/{id}/export`) — the
      blob-URL download path in `web/src/lib/api.ts`.
- [ ] Complete a **SAML SSO** into a registered SP. Exercises the two crewjam
      auto-submit script hashes and the relaxed `form-action https:`.
- [ ] Complete a **SAML single logout**. Exercises DTP's own auto-submit hash.
- [ ] Complete an **OIDC** authorization + consent round trip.
- [ ] Pay an invoice through the gateway flow.

If any item above produces a violation, roll the policy back to report-only
(both headers) rather than leaving a broken flow in place, fix the policy, and
re-run the list.

Notes on the policy's compromises, so they are not mistaken for oversights:

- `style-src 'unsafe-inline'` stays. Tailwind plus React `style={{}}` props make
  removing it impractical, and inline *style* is a far weaker vector than inline
  script. Tighten later with `style-src-attr`.
- The three `'sha256-…'` script sources are the SAML auto-submit scripts — two
  from crewjam, one from DTP's `postFormTemplate`. They are pinned by
  `TestPostBindingScriptsAreAllowlistedByCSP` and
  `TestCrewjamPostBindingScriptsAreAllowlistedByCSP`, which render the real pages
  and fail if a hash drifts from the code. **Never hand-edit a hash** — change
  the script, run the test, and paste the hash it prints.
- `frame-ancestors 'none'` supersedes `X-Frame-Options` for modern browsers; both
  are kept for old ones.
- There is **no report sink**. Violations go to the browser console only, which
  is enough for the scripted pass above. Collecting them from real users needs an
  unauthenticated POST endpoint DTP does not have yet — new attack surface that
  should not be added just to run this checklist.

### E. Infra & operational — **quarterly**

- [ ] Scan built container images (**Trivy**) and IaC/deploy config
      (**Checkov**/**tfsec**) if/when containerized.
- [ ] Secrets management — KEK, DB creds, provider tokens are injected, never
      committed (gitleaks covers the code; verify runtime injection too).
- [ ] TLS config and security headers (HSTS, CSP, X-Content-Type-Options) on the
      deployed edge.

---

## Reference standards (methodology, not tools)

- **OWASP ASVS** — Application Security Verification Standard: use as the
  requirements checklist / definition of "tested enough."
- **OWASP WSTG** — Web Security Testing Guide: the how-to for each test above.
- **OWASP Top 10** — the Semgrep `owasp-top-ten` ruleset maps to this.
- **OWASP Cheat Sheet Series** — per-topic hardening guidance.

## Cadence summary

| When | What |
|---|---|
| Every PR | `scripts/security.sh --strict` (SAST, SCA, secrets, lint, authz tests, a11y) + `/security-review` |
| Nightly | ZAP DAST, osv-scanner, CodeQL |
| Every release | Manual §A (access control) + §D (business logic) |
| Quarterly | §B OIDC conformance, §C (Burp), §E (infra) |
| Annually | §B SAML exploratory mutation (the fixed XSW/XXE table runs per-scan) |

§B's JWT attacks and the mechanical SAML XSW/XXE set are no longer manual — see
the gating checks `jwt-attacks`, `jwt-attacks-sdk` and `saml-xsw-xxe`.

Task state (last done, next due, overdue) is tracked in `security/tasks.json` and
rendered on the dashboard. Record a completed run with:

```
go run ./cmd/security-dashboard task done <id> --owner <name> --note "<what you found>"
```
