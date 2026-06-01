# Public Art Phase 3A Select + Cache Implementation Plan

> **For Hermes:** Use test-driven-development for each code task. Keep this milestone small: selected artwork + simple cache + UI select button. Do not implement full custom composition yet.

**Goal:** Let the user choose a searched public-art candidate and have `/image/public_art` reliably serve that selected artwork from a local cache.

**Architecture:** Store selected artwork separately from search config in `model.Setting{Key: "public_art_selected_candidate"}`. Extend `publicart.Service` so `FetchImage()` prefers the selected candidate, downloads it through the existing safe image downloader, and caches raw downloaded image bytes on disk to minimize CPU/storage overhead, decoding only when actively serving the image. Add authenticated handler endpoints for selecting/clearing the candidate and update the Public Art search grid with `Display on frame` / `Clear selection` actions.

**Tech Stack:** Go backend (`backend/internal/publicart`, `backend/internal/handler`, `backend/internal/service`), GORM settings, Echo HTTP handlers, Vue/Vuetify frontend (`webapp/src/components/Settings.vue`).

---

## Scope

### In scope
- Backend selected-candidate persistence via settings.
- API to select and clear the selected candidate.
- `/image/public_art` prefers selected candidate over query-based ranked search.
- Basic local disk cache for downloaded source artwork.
- Frontend button on each search result: `Display on frame`.
- Frontend button to clear the selected artwork and resume query-based rotation.
- Tests for selected candidate behavior, handler behavior, and cache hit behavior.

### Out of scope for Phase 3A
- Full custom compose dialog.
- Pan/zoom controls.
- Multi-provider expansion beyond current AIC provider.
- Google Arts import.
- Messaging commands.

## Data contract

### New setting key
```go
const SettingsKeySelectedCandidate = "public_art_selected_candidate"
```

### Selected artwork JSON
Use the current `publicart.Candidate` shape directly for Phase 3A:

```json
{
  "provider": "aic",
  "id": "aic:123",
  "title": "...",
  "artist": "...",
  "date": "...",
  "image_url": "https://...",
  "source_url": "https://...",
  "width": 2000,
  "height": 1500
}
```

Future Phase 3B can wrap this in `{ candidate, composition }`; for 3A keep storage minimal but isolate it under its own setting key so migration is easy.

---

## Task 1: Add selected-candidate settings helpers

**Objective:** Add typed helpers for loading/saving/clearing selected artwork.

**Files:**
- Modify: `backend/internal/publicart/config.go`
- Test: `backend/internal/publicart/config_test.go`

**Steps:**
1. Add `SettingsKeySelectedCandidate`.
2. Add interfaces:
   ```go
   type SettingsSetter interface { Set(key string, value string) error }
   type SettingsStore interface { SettingsGetter; SettingsSetter }
   ```
3. Add helper functions:
   - `LoadSelectedCandidate(settings SettingsGetter) (Candidate, bool, error)`
   - `SaveSelectedCandidate(settings SettingsSetter, candidate Candidate) error`
   - `ClearSelectedCandidate(settings SettingsSetter) error` using empty string.
4. Validation: saving requires non-empty `Provider`, `ID`, and `ImageURL`.
5. Tests:
   - missing/empty setting returns `(Candidate{}, false, nil)`.
   - valid JSON returns candidate and true.
   - invalid JSON returns error.
   - save rejects missing image URL.

**Verification command:**
```bash
docker run --rm -v "$PWD":/app -w /app golang:alpine sh -lc 'apk add --no-cache build-base git >/dev/null && /usr/local/go/bin/go test ./backend/internal/publicart'
```

---

## Task 2: Extend service with selected candidate + disk cache

**Objective:** Make `FetchImage()` prefer selected artwork and avoid repeated image downloads.

**Files:**
- Modify: `backend/internal/publicart/service.go`
- Test: `backend/internal/publicart/service_test.go`

**Steps:**
1. Extend `ServiceOptions`:
   ```go
   Settings SettingsGetter
   CacheDir string
   ```
2. Add fields to `Service` for selected-candidate settings and cache directory.
3. Ensure `CacheDir` exists with `os.MkdirAll(cacheDir, 0755)` before writes. If creation fails, log/return no cache error only for cache operations and still serve the downloaded image in memory.
4. In `FetchImage()`:
   - Try `LoadSelectedCandidate(settings)` when settings is non-nil.
   - If selected exists, use it directly.
   - Otherwise keep existing query/rank behavior.
5. Add `fetchCandidateImage(candidate Candidate) (image.Image, error)`:
   - Check cache first.
   - On miss, call existing downloader.
   - Save raw HTTP response bytes atomically, then decode from bytes for the active request.
6. Cache key:
   - SHA-256 of `candidate.Provider + "\x00" + candidate.ID + "\x00" + candidate.ImageURL`.
   - Store as `<cacheDir>/<sha256>.img` because the provider may return JPEG/PNG/GIF and decoding is content-based.
7. Cache behavior:
   - If `CacheDir` is empty, behave as today with no cache.
   - If cache read fails/decode fails, ignore bad cache and re-download.
   - Write to temp file then rename.
   - Avoid unbounded growth in this milestone by deleting stale cache files after successfully writing a new selected artwork cache entry. Future Phase 3B can replace this with TTL/LRU once multiple selected/composed variants exist.
   - Concurrent cold requests may redundantly download once each; atomic writes prevent corruption. Singleflight can wait until later unless this shows up in HA logs.
8. Tests:
   - selected candidate bypasses provider search.
   - cache hit avoids HTTP image request.
   - bad cache falls back to download.
   - cache directory creation failure or read-only cache falls back to serving downloaded image in memory.

**Verification command:** same `go test ./backend/internal/publicart`.

---

## Task 3: Wire cache directory in main

**Objective:** Ensure production/add-on runtime stores cache under app data dir.

**Files:**
- Modify: `backend/main.go`

**Steps:**
1. Locate `publicart.NewService(...)` call.
2. Pass:
   ```go
   Settings: settingsService,
   CacheDir: filepath.Join(dataDir, "public_art_cache"),
   ```
3. Ensure `path/filepath` import exists.

**Verification command:**
```bash
docker build --build-arg BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.21 -t esp32-photoframe-server:public-art-phase3a-test .
```

---

## Task 4: Add select/clear HTTP API

**Objective:** Let UI persist a selected artwork.

**Files:**
- Modify: `backend/internal/handler/publicart.go`
- Test: `backend/internal/handler/publicart_test.go`
- Modify route registration in `backend/main.go`.

**API:**
```http
POST /api/public-art/select
Content-Type: application/json

{
  "candidate": { ...publicart.Candidate... }
}
```

For clear:
```http
DELETE /api/public-art/select
```

**Steps:**
1. Extend `PublicArtHandler` to accept a settings store dependency.
2. Add request type:
   ```go
   type PublicArtSelectRequest struct { Candidate publicart.Candidate `json:"candidate"` }
   ```
3. Add `Select(c echo.Context) error`:
   - bind JSON.
   - validate via `SaveSelectedCandidate`.
   - return `200 {"status":"selected"}`.
4. Add `ClearSelection(c echo.Context) error`:
   - clear selected candidate.
   - return `200 {"status":"cleared"}`.
5. Register both routes explicitly under the authenticated `protectedApi` group in `backend/main.go`:
   ```go
   protectedApi.POST("/public-art/select", publicArtHandler.Select)
   protectedApi.DELETE("/public-art/select", publicArtHandler.ClearSelection)
   ```
6. Add tests:
   - valid candidate saves exact JSON.
   - invalid/missing image URL returns 400.
   - clear writes empty setting.

**Verification command:**
```bash
docker run --rm -v "$PWD":/app -w /app golang:alpine sh -lc 'apk add --no-cache build-base git >/dev/null && /usr/local/go/bin/go test ./backend/internal/handler'
```

---

## Task 5: Add frontend Display on frame button

**Objective:** Let user select or clear a candidate from the preview grid.

**Files:**
- Modify: `webapp/src/components/Settings.vue`

**Steps:**
1. Add a `Display on frame` button to each Public Art candidate card.
2. Add state: `publicArtSelectingId = ref('')`.
3. Add function:
   ```ts
   const selectPublicArtCandidate = async (candidate: PublicArtCandidate) => {
     publicArtSelectingId.value = candidate.id;
     try {
       await api.post('/public-art/select', { candidate });
       showMessage('Public art selection saved. Frames using /image/public_art will show this artwork.');
     } catch (e: any) {
       showMessage('Failed to select artwork: ' + (e.response?.data?.error || e.message), true);
     } finally {
       publicArtSelectingId.value = '';
     }
   };
   ```
4. Disable button while selecting; show spinner on the selected card.
5. Add a `Clear Selection` button near the Search preview title:
   ```ts
   await api.delete('/public-art/select')
   ```
   Success message: `Public art selection cleared. Frames will use the default search query again.`
6. Keep `Save Public Art Settings` separate; selecting a candidate should not require saving query config first.

---

## Gemini review notes incorporated

Gemini reviewed this plan headlessly and flagged four changes now incorporated above:
- register new mutation routes under authenticated `protectedApi` explicitly;
- cache raw image bytes instead of re-encoding decoded images as PNG;
- add cache directory creation/failure behavior and minimal stale-cache cleanup;
- add a clear-selection UI/API path so users can return to query-based rotation.

**Verification command:**
```bash
cd webapp && npm run build
```

---

## Task 6: Final verification and commit

**Objective:** Prove the feature works and push a clean branch.

**Commands:**
```bash
docker run --rm -v "$PWD":/app -w /app golang:alpine sh -lc 'apk add --no-cache build-base git >/dev/null && /usr/local/go/bin/go test ./backend/internal/publicart ./backend/internal/service ./backend/internal/handler'
cd webapp && npm run build
docker build --build-arg BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.21 -t esp32-photoframe-server:public-art-phase3a-test .
git diff --check
git status --short
git add backend webapp/src/components/Settings.vue docs/plans/public-art-phase-3a-select-cache.md
git commit -m "feat: allow selecting public art artwork"
git push MarcusTseng public-art-source
git push MarcusTseng public-art-source:main
```

**Expected:**
- Go tests pass.
- Frontend build passes.
- Docker build passes.
- Remote `main` and `public-art-source` point to the new commit.
