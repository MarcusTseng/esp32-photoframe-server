# Phase 3B — Compose / Crop / Pan UI + Backend Composition

## Goal

Allow users to manually compose a selected artwork for the target frame before saving: cover / fit / custom scale modes, zoom, pan, background color. UI preview must match backend-served output exactly.

## Architecture

### Data Model

**`SelectedArtwork`** replaces storage of raw `Candidate` alone:

```go
// backend/internal/publicart/types.go
type Composition struct {
    ScaleMode       string  `json:"scale_mode"`        // "cover" | "fit" | "custom"
    Zoom            float64 `json:"zoom"`              // 1.0 = 100%, 2.0 = 200%
    PanX            float64 `json:"pan_x"`             // -0.5 to 0.5 (fraction of image center shift)
    PanY            float64 `json:"pan_y"`
    BackgroundColor string  `json:"background_color"`  // e.g. "white", "black", "#1a1a1a"
}

type SelectedArtwork struct {
    Candidate   Candidate   `json:"candidate"`
    Composition Composition `json:"composition"`
}
```

### Storage

- `public_art_selected_candidate` setting key is renamed conceptually to `public_art_selected_artwork` but key stays for backward compat; stores `SelectedArtwork` JSON.
- On read: if existing value is plain `Candidate` (no `Composition` field), default `Composition` with `ScaleMode: "cover"` is assumed (backward compat).
- On write: always writes `SelectedArtwork`.

### Backend Composition Math

All composition happens server-side using the stdlib `image` package.

**Cover mode** (default): fill the target rectangle, crop overflow
```go
// Source image is scaled so the cropped region covers the full target.
// croppedW = sourceW * zoom
// croppedH = sourceH * zoom
// offsetX = (croppedW - targetW) / 2 + panX * croppedW
// offsetY = (croppedH - targetH) / 2 + panY * croppedH
```

**Fit mode**: scale to fit inside target, letterbox with background color
```go
// Scale so image fits entirely within targetW × targetH
// Letterbox: place centered, fill remaining with BackgroundColor
```

**Custom mode**: same as cover but with explicit zoom + pan values stored.

### API Changes

**`POST /api/public-art/preview`**

Request:
```json
{
  "candidate": { ... },
  "composition": { "scale_mode": "cover", "zoom": 1.0, "pan_x": 0, "pan_y": 0, "background_color": "white" },
  "target_width": 1200,
  "target_height": 1600
}
```

Response: JPEG image (200×200 max for preview speed). Returns 400 on bad input.

**`POST /api/public-art/select`** — updated to accept `SelectedArtwork`:
```json
{
  "candidate": { ... },
  "composition": { "scale_mode": "cover", "zoom": 1.0, "pan_x": 0, "pan_y": 0, "background_color": "white" }
}
```

**`GET /image/public_art`** — updated to apply stored `Composition`:
- Load `SelectedArtwork` (candidate + composition)
- Fetch + decode image
- Apply `ComposeImage(img, composition, targetW, targetH)` using device width/height from request
- Return composed image

### Frontend: Compose Dialog

Trigger: click "Display on frame" on a candidate card.

The dialog contains:
1. **Target device selector** (defaults to first frame, shows resolution)
2. **Scale mode buttons**: `Cover` | `Fit` | `Custom`
3. **Zoom slider**: 0.5× – 3.0× (only active in Custom mode, cover shows current zoom as preview hint)
4. **Pan control**: two sliders for X/Y offset, or drag on preview image (Custom mode)
5. **Background color**: color picker or preset (white, black, #1a1a1a, auto) — only shown in Fit mode
6. **Preview canvas**: shows the actual composed result fetched from `/api/public-art/preview`
7. **Actions**: `Cancel` | `Save & Display on Frame`

**Preview flow**:
- On scale/zoom/pan/background change (debounced 300ms), call `POST /api/public-art/preview`
- Update preview canvas with returned image
- On `Save & Display on Frame`: call `POST /api/public-art/select` with candidate + composition

### Files to Change

**Backend:**
- `backend/internal/publicart/types.go` — add `Composition` and `SelectedArtwork`
- `backend/internal/publicart/compose.go` — new file: `ComposeImage()` function
- `backend/internal/publicart/compose_test.go` — composition math tests
- `backend/internal/publicart/config.go` — update `SaveSelectedCandidate` to store `SelectedArtwork`; `LoadSelectedCandidate` returns `SelectedArtwork`
- `backend/internal/publicart/config_test.go` — update tests
- `backend/internal/publicart/service.go` — `FetchImage(targetW, targetH int, composition Composition)` applies composition
- `backend/internal/handler/publicart.go` — add `PreviewRequest` struct, `Preview` handler, update `Select` to accept `SelectedArtwork`
- `backend/internal/handler/publicart_test.go` — add preview + select tests
- `backend/internal/service/imagesources.go` — pass request width/height to `FetchImage`
- `backend/main.go` — update routes

**Frontend:**
- `webapp/src/components/Settings.vue` — add compose dialog, update select flow
- `webapp/src/api.ts` — add `previewPublicArt(candidate, composition, targetW, targetH)` API call

## Test Scenarios

1. `TestComposeCoverScalesAndCrops` — source 2000×1500, target 1200×1600, zoom 1.0, expect output 1200×1600
2. `TestComposeCoverWithZoom` — zoom 1.5, expect tighter crop
3. `TestComposeCoverWithPan` — pan_x=0.2, expect visible shift
4. `TestComposeFitLetterboxes` — source 2000×1500, target 1200×1600, expect output 1200×1600 with letterbox
5. `TestComposeFitBackgroundColor` — verify background color is applied
6. `TestPreviewEndpointReturnsJPEG` — POST valid candidate + composition, expect 200 + image/jpeg
7. `TestSelectStoresComposition` — POST selected artwork, GET, verify composition round-trip
8. `TestFetchImageAppliesComposition` — service-level test with stored composition

## Verification

1. `go test ./backend/internal/publicart/...` — all pass
2. `npm run build` — no TypeScript errors
3. `docker build` — no compilation errors
4. Manual: open Settings → Public Art → Search → click "Display on frame" → compose dialog appears → adjust controls → preview updates → save → `/image/public_art` returns composed image matching preview
