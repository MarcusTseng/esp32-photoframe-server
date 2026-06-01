# Public Art Phase 4 — Stabilize HA UI, Thumbnails, Selection, and Providers

> **For Hermes:** Do not implement feature work before completing Task 1 diagnostics. The current user-visible bug is that the Home Assistant add-on UI still shows no search-result preview thumbnails and no visible `Preview & crop` / `Use this artwork` buttons, even though the repo's latest `Settings.vue` contains those controls. Treat this as a deployment/runtime/static-asset verification problem first, not as a CSS guess.

**Goal:** Make the Public Art search flow reliable in the actual Home Assistant add-on UI, then improve artwork quality by adding stable server-side thumbnail/proxy behavior and a second provider.

**Architecture:** First add explicit runtime/build observability so we can prove which backend commit/version and frontend bundle the add-on is serving. Then fix the search card rendering using a server-owned thumbnail endpoint that is safe behind HA ingress and does not depend on browser access to external art CDNs. Finally, add a provider abstraction path for sources less likely to be blocked than AIC IIIF.

**Tech Stack:** Go backend (`backend/main.go`, `backend/internal/handler`, `backend/internal/publicart`), Echo routes, Vue/Vuetify frontend (`webapp/src/components/Settings.vue`), Vite build, Home Assistant add-on config (`config.yaml`), Docker add-on build.

---

## Current Status / Evidence

### Implemented in repo

Latest relevant commits currently in `main` / `public-art-source`:

- `951539a fix: proxy public art thumbnails through server`
- `8801615 chore: bump public-art add-on version to v1.7.5-public-art.2`
- `8bd1af1 feat: rank public art by frame orientation`
- `c321c78 fix: show AIC thumbnails when IIIF images are blocked`
- `f907c0a feat: add compose/crop UI panel with live preview`

Current `config.yaml` version:

```yaml
version: "v1.7.5-public-art.3"
```

Current repo `Settings.vue` search cards include:

- `publicArtThumbnailUrl(candidate)` on `<v-img>`
- `Preview & crop` button
- `Use this artwork` button

Current backend routes include:

```go
protectedApi.POST("/public-art/search", pah.Search)
protectedApi.POST("/public-art/select", pah.Select)
protectedApi.DELETE("/public-art/select", pah.ClearSelection)
protectedApi.GET("/public-art/thumbnail", pah.Thumbnail)
protectedApi.POST("/public-art/preview", pah.Preview)
```

### User-visible bug still present

From Marcus's HA add-on UI:

- Search returns public-art result cards.
- Result preview images are still not visible.
- `Preview & crop` / select-style buttons are not visible.

### Working hypotheses, ordered by likelihood

1. **Installed add-on is not actually serving the latest frontend bundle.** If the buttons are absent, the browser may be running an older `assets/index-*.js`, or the add-on did not rebuild/reinstall despite repo changes.
2. **Browser/HA ingress is serving cached `index.html` or cached JS/CSS.** The backend serves static assets without explicit no-cache headers for `index.html`; Vite asset filenames are hashed, but stale `index.html` can point to an older bundle.
3. **HA add-on repository version/build cache did not notice the fork version bump or did not rebuild the image.** Need verify add-on version shown in HA, container image ID/build time, and served bundle content.
4. **Thumbnail endpoint is unavailable in the installed backend.** If `/api/public-art/thumbnail` returns HTML, 401, 404, or old backend behavior, thumbnails will not show.
5. **CSS/layout issue hides card actions.** Lower likelihood because buttons are absent entirely from the user's view, but still test after proving latest bundle is loaded.

---

## Acceptance Criteria

Phase 4 is complete only when all of these are true in the actual HA add-on UI, not just local builds:

1. Settings → Public Art → Search shows result cards with visible preview images or an explicit per-card fallback/error state.
2. Every result card visibly shows two actions:
   - `Preview & crop`
   - `Use this artwork`
3. Clicking `Preview & crop` opens/shows compose controls and a preview image or a clear error.
4. Clicking `Use this artwork` saves the artwork selection without requiring preview first.
5. `/image/public_art` returns the selected/composed image for a frame-sized request.
6. HA add-on UI exposes a build/version marker proving the deployed frontend/backend version matches the repo commit being tested.
7. A fresh browser session or hard refresh is not required for normal users after future add-on upgrades.

---

## Task 1: Prove what HA add-on is actually serving

**Objective:** Add and/or use diagnostics to identify backend version, frontend bundle version, and route availability in the installed add-on.

**Files:**

- Modify: `backend/main.go`
- Modify or create: `backend/internal/handler/system.go` if a system handler already exists; otherwise add a small status handler near main route setup.
- Modify: `webapp/src/components/Settings.vue`
- Modify: `Dockerfile`

**Steps:**

1. Add build args/envs in Dockerfile:

   ```dockerfile
   ARG APP_VERSION=dev
   ARG GIT_COMMIT=unknown
   ENV APP_VERSION=$APP_VERSION
   ENV GIT_COMMIT=$GIT_COMMIT
   ```

2. Add a lightweight protected or public endpoint:

   ```http
   GET /api/build-info
   ```

   Response:

   ```json
   {
     "version": "v1.7.5-public-art.4",
     "git_commit": "...",
     "static_dir": "/app/static"
   }
   ```

3. Add a tiny build marker in the frontend, near the Public Art settings section or page footer:

   ```text
   Public Art build: v1.7.5-public-art.4 / <short sha>
   ```

4. Add a debug line in the Public Art section showing:

   - number of candidates loaded;
   - first candidate id;
   - whether `thumbnail_url` is present;
   - generated thumbnail endpoint path.

5. Verify against local Docker container first:

   ```bash
   docker build \
     --build-arg BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.21 \
     --build-arg APP_VERSION=v1.7.5-public-art.4 \
     --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
     -t esp32-photoframe-server:phase4-build-info .
   ```

6. Run container locally and curl:

   ```bash
   docker run --rm -p 9607:9607 esp32-photoframe-server:phase4-build-info
   curl -s http://localhost:9607/api/build-info | jq .
   curl -s http://localhost:9607/ | grep -o 'assets/index-[^" ]*\.js' | head
   ```

**Expected:** We can prove the backend and frontend bundle version before debugging thumbnails/buttons.

**Commit:**

```bash
git add Dockerfile backend webapp/src/components/Settings.vue
git commit -m "chore: expose add-on build info"
```

---

## Task 2: Fix frontend cache behavior for HA add-on upgrades

**Objective:** Ensure users do not keep stale `index.html` or stale frontend bundles after an add-on upgrade.

**Files:**

- Modify: `backend/main.go`
- Test: add HTTP handler test if route setup is testable; otherwise verify with curl against local container.

**Steps:**

1. Replace `e.File("/", ...)` and SPA fallback with handlers that set no-cache headers for `index.html`:

   ```go
   func serveIndex(c echo.Context) error {
       c.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
       c.Response().Header().Set("Pragma", "no-cache")
       return c.File(filepath.Join(staticDir, "index.html"))
   }
   ```

2. Keep `/assets` cacheable because Vite asset filenames are hashed.

3. Local verification:

   ```bash
   curl -I http://localhost:9607/ | grep -i cache-control
   curl -I http://localhost:9607/assets/<actual-index-js> | grep -i cache-control || true
   ```

**Expected:** `index.html` is not cached across upgrades.

**Commit:**

```bash
git add backend/main.go
git commit -m "fix: prevent stale frontend after add-on upgrades"
```

---

## Task 3: Make search card rendering self-diagnostic and robust

**Objective:** If thumbnails cannot load, show a visible fallback/error instead of an empty gray card, and keep buttons always visible.

**Files:**

- Modify: `webapp/src/components/Settings.vue`
- Test: `webapp` build; optional Playwright script against local server.

**Steps:**

1. Add `error` slot to `<v-img>`:

   ```vue
   <template #error>
     <div class="d-flex flex-column align-center justify-center h-100 text-caption text-medium-emphasis pa-3">
       <v-icon icon="mdi-image-broken-variant" class="mb-1" />
       Preview unavailable
     </div>
   </template>
   ```

2. Add `placeholder` slot to show loading state.

3. Keep `v-card-actions` outside metadata and always rendered.

4. If candidate has no usable image/thumbnail URL, disable only preview but leave source link visible.

5. Run:

   ```bash
   cd webapp && npm run build
   ```

**Expected:** Even if AIC/CDN fetch fails, user sees a clear fallback and visible actions.

**Commit:**

```bash
git add webapp/src/components/Settings.vue
git commit -m "fix: make public art cards visible on thumbnail failures"
```

---

## Task 4: Verify thumbnail endpoint through browser/ingress path

**Objective:** Prove `/api/public-art/thumbnail` works from the same origin/path the frontend uses.

**Files:**

- Modify: `backend/internal/handler/publicart.go` only if diagnostics show route/auth/path problems.
- Test: `backend/internal/handler/publicart_test.go`

**Steps:**

1. Use search response to capture first candidate.
2. Construct exact frontend thumbnail URL:

   ```text
   /api/public-art/thumbnail?candidate_image_url=...&candidate_thumbnail_url=...
   ```

3. Curl inside authenticated/local context and inspect:

   ```bash
   curl -i '<thumbnail-url>' | head
   ```

4. Expected good response:

   ```text
   HTTP/1.1 200 OK
   Content-Type: image/jpeg
   ```

5. If response is `401` or login HTML, decide whether thumbnail should be under protected API or a public image route with signed/candidate hash. Prefer **protected first** if cookies are present; only move to public route if HA ingress strips auth for image tags.

**Expected:** Thumbnail endpoint works from actual browser origin.

**Commit if changed:**

```bash
git add backend/internal/handler/publicart.go backend/internal/handler/publicart_test.go backend/main.go
git commit -m "fix: serve public art thumbnails through HA ingress"
```

---

## Task 5: Add Playwright regression for Public Art card UI

**Objective:** Prevent future regressions where results render but images/buttons disappear.

**Files:**

- Create: `webapp/tests/public-art-cards.spec.ts` or `tests/e2e/public-art-cards.py` depending on existing test tooling.
- Optionally create: `backend/internal/handler/publicart_fixture_test.go` only if a fixture mode is needed.

**Steps:**

1. Start backend/frontend locally with fixture/mocked public-art search data.
2. Navigate to Settings → Public Art.
3. Trigger Search.
4. Assert:

   ```text
   candidate cards count > 0
   text "Preview & crop" visible
   text "Use this artwork" visible
   card image/fallback container visible
   ```

5. Capture screenshot artifact on failure.

**Verification:**

```bash
npm run build
# plus chosen Playwright command
```

**Expected:** UI card controls are covered by an automated browser test.

---

## Task 6: Add a second provider after UI is stable

**Objective:** Reduce reliance on AIC IIIF, which is currently Cloudflare-challenged for direct images.

**Provider preference:** Rijksmuseum first, then Met/Wikimedia if needed.

**Files:**

- Modify: `backend/internal/publicart/types.go`
- Create: `backend/internal/publicart/rijksmuseum.go`
- Test: `backend/internal/publicart/rijksmuseum_test.go`
- Modify: `backend/internal/publicart/service.go` or provider registry
- Modify: `webapp/src/components/Settings.vue` if provider selector is exposed

**Steps:**

1. Keep current `Candidate` contract.
2. Add provider constant:

   ```go
   const ProviderRijksmuseum = "rijksmuseum"
   ```

3. Implement search returning:

   - provider id;
   - title / artist / date;
   - `image_url` from stable full image field;
   - `thumbnail_url` from stable web image thumbnail if available;
   - source URL;
   - dimensions.

4. Add tests with httptest JSON fixture.
5. Add config/provider selection only after backend provider works.

**Expected:** Search can return useful candidates even when AIC images are blocked.

---

## Task 7: Release / HA add-on upgrade checklist

**Objective:** Make sure the fix reaches Marcus's HA add-on, not just GitHub.

**Steps:**

1. Bump fork version only:

   ```yaml
   version: "v1.7.5-public-art.4"
   ```

   Keep upstream base `v1.7.5`; increment only `public-art.#`.

2. Push to both branches:

   ```bash
   git push MarcusTseng public-art-source
   git push MarcusTseng public-art-source:main
   ```

3. In HA:

   - Reload add-on repository.
   - Confirm add-on version shows `v1.7.5-public-art.4` or later.
   - Rebuild/reinstall add-on if HA uses local build cache.
   - Restart add-on.

4. In browser:

   - Open add-on in a private/incognito window once.
   - Verify visible build marker.
   - Search Public Art.
   - Confirm thumbnails/fallbacks and buttons visible.

5. If build marker is old, stop debugging UI and fix HA deployment/cache first.

---

## Do Not Do Yet

- Do not add more image providers before proving the deployed UI is current.
- Do not chase CSS guesses before confirming the loaded bundle contains `Preview & crop` and `Use this artwork`.
- Do not bump upstream version to `v1.7.6`; use `v1.7.5-public-art.N` only.
- Do not make firmware changes for this problem.

---

## Verification Commands Summary

```bash
# Backend tests
docker run --rm -v "$PWD":/app -w /app golang:alpine sh -lc 'apk add --no-cache build-base git >/dev/null 2>&1 && /usr/local/go/bin/go test ./backend/internal/publicart/... ./backend/internal/service/... ./backend/internal/handler/... -v 2>&1'

# Frontend build
cd webapp && npm run build

# Full add-on Docker build
docker build \
  --build-arg BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.21 \
  --build-arg APP_VERSION=v1.7.5-public-art.4 \
  --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
  -t esp32-photoframe-server:phase4 .
```

---

## Execution Order

1. Task 1 — Build/version diagnostics.
2. Task 2 — Static cache fix.
3. Task 3 — Robust card rendering.
4. Task 4 — Thumbnail endpoint ingress verification.
5. Task 5 — Browser regression test.
6. Task 7 — Release/check add-on upgrade path.
7. Task 6 — Add second provider after UI is proven stable.
