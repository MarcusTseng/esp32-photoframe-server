# Public Art — Feature Status & Remaining Work Inventory

> Last updated: 2026-06-02

## Completed ✅

| Phase | What | Status |
|-------|------|--------|
| Phase 1 | AIC metadata + provider + ranking | ✅ Done |
| Phase 2 | Backend `/image/public_art` MVP + config | ✅ Done |
| Phase 3A | Select candidate + disk cache + UI select button | ✅ Done |
| Phase 3B (core) | `ComposeImage` math, `Composition`/`SelectedArtwork` types, Preview API, compose panel state in Settings.vue | ✅ Core done |
| Phase 4 | Build diagnostics, no-cache index, robust thumbnail rendering, public thumbnail endpoint | ✅ Done through `.6` |
| Public Art dedup | Per-device serving history with configurable repeat window | ✅ Done in `.6` |

---

## Phase 4 — UI Stabilization (Mostly Complete)

Tasks 1–4 were completed across `.5` and `.6`. Tasks 5–6 remain future improvements.

### Task 1: Build info endpoint (DONE in v1.7.5-public-art.6)
Add `GET /api/build-info` + frontend build version marker so we can prove what the HA add-on is actually serving.

### Task 2: Prevent stale frontend after upgrades (DONE in v1.7.5-public-art.6)
`index.html` needs no-cache headers; `/assets/*` stays cacheable (Vite uses hashed filenames).

### Task 3: Robust card rendering (DONE in v1.7.5-public-art.6)
Public Art candidate cards now use a plain eager `<img>` with explicit fallback instead of Vuetify lazy image rendering, so HA ingress does not leave grey placeholder thumbnails indefinitely.

### Task 4: Thumbnail endpoint through ingress (DONE in v1.7.5-public-art.5)
`/api/public-art/thumbnail` moved from `protectedApi` to public routes.

### Task 5: Playwright regression test (NOT DONE)
Browser-level test for Public Art cards, thumbnails, buttons visibility.

### Task 6: Second provider (NOT DONE)
Add Rijksmuseum as fallback provider (AIC IIIF is frequently blocked).

### Task 7: HA add-on release checklist (ONGOING)
Verify HA add-on serves latest bundle.

---

## Phase 3B — Compose UI (Partial)

Backend compose math is complete and tested:
- `backend/internal/publicart/compose.go` — `ComposeImage()` for cover/fit/custom
- `backend/internal/publicart/compose_test.go` — 8 composition tests pass
- `backend/internal/handler/publicart.go` — `Preview` handler accepts `PublicArtPreviewRequest`
- `Settings.vue` — `openComposePanel`, `selectPublicArtCandidate`, `usePublicArtCandidate`, `publicArtComposingId`, `publicArtComposition`

If the compose panel still is not visible in the HA add-on UI after `.6`, check `/api/build-info` first to confirm the running add-on version/commit, then inspect browser console/network for the Public Art card buttons and thumbnail requests.

---

## Dedup: 24-hour Serving History (DONE in v1.7.5-public-art.6)

**Requested 2026-06-02 by MarcusCST:** Track which artworks have been served to a device in the last N hours, so the same artwork doesn't repeat on auto-rotate cycles.

### Design

**New table:**
```sql
CREATE TABLE public_art_serving_history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id   INTEGER NOT NULL,
    source      VARCHAR(32) NOT NULL,
    artwork_id  VARCHAR(128) NOT NULL,
    served_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_public_art_history_device ON public_art_serving_history(device_id, served_at DESC);
```

**New setting:**
- `public_art_dedup_hours` — hours before served artwork can repeat; default `24`; set to `0` to disable.

### Logic

In `publicart.Service.FetchImageWithComposition()`:

```
1. Load selected candidate → if set, use it directly (selected takes priority).
2. Else: query AIC, get ranked candidates (limit 10).
3. For each ranked[i]:
     - Check: has this device seen artwork_id within public_art_dedup_hours?
     - If YES: skip, continue to ranked[i+1]
     - If NO: use it
4. If all candidates were recently served: allow repeat (log warning).
5. After choosing candidate: write to public_art_serving_history.
```

**Dedup check query:**
```sql
SELECT 1 FROM public_art_serving_history
  WHERE device_id = ? AND artwork_id = ?
  AND served_at > datetime('now', '-' || ? || ' hours')
  LIMIT 1;
```

**Automatic cleanup:**
- On each `FetchImage`, also purge rows older than `public_art_dedup_hours` for this device (keep table small).
- Or use a background cleanup goroutine on startup / every hour.

### Files

| File | Change |
|------|--------|
| `backend/internal/model/model.go` | Add `PublicArtServingHistory` model |
| `backend/internal/db/db.go` | Add `PublicArtServingHistory` to migrations |
| `backend/internal/service/settings.go` | Add `public_art_dedup_hours` setting |
| `backend/internal/publicart/config.go` | Add `DedupHours() int` method |
| `backend/internal/publicart/service.go` | Add dedup check + history write in `FetchImageWithComposition` |
| `backend/internal/publicart/service_test.go` | Test: recently served artwork is skipped; all-skip allows repeat |
| `backend/internal/handler/publicart_test.go` | No handler changes needed |
| `webapp/src/components/Settings.vue` | Expose `public_art_dedup_hours` in Public Art settings tab |

### API

No new endpoint needed — dedup is transparent to the API. Optional:
- `GET /api/public-art/stats` (bonus) — show how many artworks are in history, oldest entry.

### Verification

```bash
# Go tests
docker run --rm -v "$PWD":/app -w /app golang:alpine sh -lc \
  'apk add --no-cache build-base git >/dev/null 2>&1 && \
   CGO_ENABLED=1 /usr/local/go/bin/go test \
   ./backend/internal/publicart/... ./backend/internal/service/... -v'

# npm build
cd webapp && npm run build

# Docker build
docker build \
  --build-arg BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.21 \
  --build-arg VERSION=v1.7.5-public-art.6 \
  -t esp32-photoframe-server:dedup-test .

# Push
git add backend webapp
git commit -m "feat: add 24-hour dedup for public art auto-rotate"
git push MarcusTseng public-art-source
git push MarcusTseng HEAD:main
```

---

## Execution Order

```
Priority 1 (UX improvements):
  → Task 5: Playwright regression test
  → Task 6: Second provider (Rijksmuseum)

Priority 2 (Polish):
  → HA add-on release + changelog bump
```