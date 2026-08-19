# SSR Evaluation: Minecraft Container Manager frontend

Status: evaluation complete. This document records the assessment and the
recommendation for the TODO item "Evaluate true server-side rendering for the
React/shadcn app (currently shipped as a static SPA embedded in the Go
binary)."

Date: 2026-08-19

## 1. Current architecture

MCM ships the panel as a single Go binary. The frontend is a React 19 +
TanStack Router + Vite single-page application (SPA) that is built to static
assets and embedded into the binary with `go:embed`.

The build pipeline (see `Dockerfile`):

- A `node:22-alpine` stage runs `pnpm build` in `web/`, producing static
  output in `web/dist/`.
- `COPY --from=web /app/web/dist ./internal/web/dist` copies that output over
  the committed placeholder.
- A `golang:1.26-alpine` stage compiles the Go binary, embedding
  `internal/web/dist` (see `internal/web/embed.go`).
- A minimal `alpine:3.20` runtime image runs the single `mcm` binary.

The same binary is deployed two ways: as a Docker image (`docker-compose.yml`)
or as a bare-metal process via `deploy/mcm.service` (systemd). Every route,
API and static alike, is served by one HTTP server (`internal/api/server.go`).

Static serving is implemented in `handleStatic`:

- Any request under `/api/` returns JSON 404 if not matched by an API route.
- Any other path is looked up in the embedded `dist` filesystem.
- If the path does not exist as a real file, it falls back to `index.html`
  so the TanStack Router can handle client-side navigation (the SPA fallback).
- Missing files after the fallback return `http.NotFound`.

Authentication is cookie and CSRF based:

- `auth.CookieName` session cookie, validated by `sessions.Validate` on every
  protected route via `requireAuth`.
- A double-submit CSRF cookie (`CSRFCookieName`) checked by middleware on all
  state-changing requests.
- TOTP and WebAuthn passkey second factors.

The frontend determines auth state entirely on the client: `AuthProvider` calls
`api.me()` on mount and maps the response (or 401/404) to one of
`loading | onboarding | unauthenticated | authenticated`. There is no server
rendered auth state today.

## 2. Evaluation criteria

The decision to move to SSR should be weighed against the properties that make
this app what it is:

**SEO value.** This is an authenticated operational panel. There are no public
content pages to index. Crawlers have nothing useful to consume; the whole
product lives behind a login. SEO-driven SSR (the most common reason to adopt
it) is essentially worthless here.

**First paint / perceived latency.** The only public-ish pages are the login
and onboarding screens. An authenticated dashboard gains nothing from SSR
because the user still has to authenticate and the data has to hydrate
client-side anyway.

**The single-binary property.** MCM's strongest operational feature is that the
entire product is one Go binary: trivial cold start, no runtime deps, simple
bare-metal systemd unit, one container, one process to watch. SSR
fundamentally threatens this.

**Auth / cookies.** SSR that renders authenticated content must negotiate the
session and CSRF cookies server-side before the JS runs. That means
duplicating cookie validation, CSRF handling, and TOTP/passkey context in the
rendering path, which is redundant and adds security surface.

**Cold starts.** A Node SSR process (no V8 module cache in a fresh container)
has slow cold starts and adds memory. A Go binary starts in milliseconds and
costs almost nothing. This matters for a panel that often runs on low-resource
hosts.

**Deployment complexity.** Adopting real SSR would mean either a Node sidecar
(violating single-binary, requiring a process/compose manager and a separate
port or proxy) or embedding a JS runtime in Go (not viable).

**Maintenance.** An SPA already ships with `hydrateRoot`, which is the
rehydration entry point. That is a signal the code is structured to be
SSR-compatible, but it does not by itself justify running SSR.

**Fault isolation.** SSR means server-side rendering failures can take down
pages for everyone. A static SPA with a JSON API has clean separation: the API
and the shell fail independently.

## 3. Options

### Option A: Stay a static SPA, harden the fallback
Recommended. Keep the embedded static build and the Go binary, and polish
`handleStatic` so real resource misses return 404 instead of a soft-404
`index.html`.

- Pros: preserves the single binary, zero deployment change, no cold-start or
  memory cost, auth stays purely cookie+CSRF handled by the Go API, lowest
  risk.
- Cons: no server-rendered HTML; login/onboarding first paint still waits for
  the JS bundle.
- Effort: very low (a few lines in one handler) plus verification.

### Option B: Vite SSR / `react-dom/server` integrated into the Go binary
Runs a Node process at request time inside the Go process, or shells out to a
Node runtime to pre-render, then serves the rendered HTML from Go.

- Pros: real server-rendered HTML for every route; SEO if it ever matters; keeps
  one exposed port if done as a build-time pre-render.
- Cons: embedding or invoking a Node runtime inside Go is not practical (no
  clean in-process JS runtime, poor cold start, heavy). Doing this as a
  build-time static pre-render is possible for a fixed route set, but for an
  authenticated panel most routes still must render client-side after auth, so
  the benefit is confined to login/onboarding.
- Effort: high and architectural; not justified by the value.

### Option C: Separate Node SSR sidecar
Run a small Node server (e.g. Vite SSR or a React server bundle) alongside the
Go binary, proxied through it.

- Pros: cleanest way to get true SSR with existing tooling; each feature uses
  the runtime it is best at.
- Cons: destroys the single-binary model; needs a process manager or a second
  compose service; adds Node cold-start and memory on low-resource hosts; must
  reproduce session/CSRF validation or proxy them; more surface area for an
  authenticated panel with little SEO upside.
- Effort: high, ongoing operational cost.

## 4. Recommendation

Do not move to true SSR. Keep the static SPA embedded in the Go binary.

Rationale, in priority order:

- There is no SEO value; this is an authenticated panel behind a login.
- The single-binary deployment is a major strength for both Docker and
  bare-metal systemd, and every SSR option erodes it.
- Auth is already cleanly solved by cookie + CSRF handled in Go; SSR would
  re-implement that server-side and add security surface.
- Cold starts, memory, and process-management complexity all argue against a
  Node runtime on top of a lean Go service.

If a future requirement ever makes SSR necessary (e.g. public status pages
that must be indexed, or a requirement to render meaningful content before
JS), the correct path is Option C (a Node sidecar) applied to only those
public routes, never to the whole app, and only after those public-facing
needs materialize. Until then, the SPA-first model is the right one.

## 5. Implemented improvement (small, low-risk)

As part of this evaluation, `handleStatic` was tightened so that static
resource misses (paths that look like files, e.g. `/assets/foo.js` or
`/favicon.ico`, that are not present in the embedded dist) return a real 404
instead of a "soft 404" that serves `index.html` with a 200. Client-side
navigation routes (`/dashboard`, `/servers/{id}`, etc.) still receive the SPA
fallback. The `/api/` miss path already returns JSON 404 and is unchanged.

Why it helps:

- A missing script, stylesheet, image, or other asset no longer silently
  renders `index.html` with the wrong content type and a 200, which is
  confusing and hurts debugging.
- The SPA router behavior is preserved for every real route.

## 6. Optional future ideas (not implemented)

- Pre-render login/onboarding as static HTML at build time for a marginally
  faster first paint on the two public screens. Low value, so only pursued if
  someone cares about those two routes' cold first load.
- Add cache headers (`Cache-Control: immutable`) for Vite's content-hashed
  `/assets/*` files and `no-cache` for `index.html`. Mildly beneficial, not
  required.
- Keep `handleStatic`'s route set in sync with the client route tree; today
  the fallback is route-agnostic, which is the correct robustness property to
  preserve.
- Reevaluate SSR if the product ever grows a public, indexable surface (e.g.
  public server status pages). At that point, revisit Option C scoped to those
  routes only.
