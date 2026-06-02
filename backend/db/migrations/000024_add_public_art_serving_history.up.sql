-- Public Art auto-rotate deduplication: keep track of which artwork
-- was served to which device so we don't repeat the same work too soon.
-- Cleanup is done in-service (deleted on fetch or by a background routine).

CREATE TABLE IF NOT EXISTS public_art_serving_history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id  INTEGER NOT NULL,
    source     TEXT NOT NULL,                          -- e.g. "aic", "rijksmuseum"
    artwork_id TEXT NOT NULL,                          -- e.g. "aic:12345"
    served_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pah_device_served
    ON public_art_serving_history (device_id, served_at DESC);