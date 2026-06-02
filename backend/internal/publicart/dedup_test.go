package publicart

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeDedupDB implements DedupHistoryDB for testing without a real DB.
type fakeDedupDB struct {
	entries []model.PublicArtServingHistory
}

func (f *fakeDedupDB) Record(deviceID uint, source, artworkID string) {
	f.entries = append(f.entries, model.PublicArtServingHistory{
		DeviceID:  deviceID,
		Source:    source,
		ArtworkID: artworkID,
		ServedAt:  time.Now(),
	})
}

func (f *fakeDedupDB) IsRecentlyServed(deviceID uint, source, artworkID string, hours int) bool {
	if hours <= 0 {
		return false
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	for _, e := range f.entries {
		if e.DeviceID == deviceID && e.Source == source && e.ArtworkID == artworkID && e.ServedAt.After(cutoff) {
			return true
		}
	}
	return false
}

func (f *fakeDedupDB) Cleanup(deviceID uint, hours int) {
	if hours <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	kept := f.entries[:0]
	for _, e := range f.entries {
		if e.DeviceID != deviceID || e.ServedAt.After(cutoff) {
			kept = append(kept, e)
		}
	}
	f.entries = kept
}

// localFakeSettingsStore is package-local, distinct from config_test.go's fakeSettingsStore.
type localFakeSettingsStore struct {
	Data map[string]string
}

func newLocalFakeSettingsStore() *localFakeSettingsStore {
	return &localFakeSettingsStore{Data: make(map[string]string)}
}

func (s *localFakeSettingsStore) Get(key string) (string, error) { return s.Data[key], nil }
func (s *localFakeSettingsStore) Set(key, value string) error     { s.Data[key] = value; return nil }

// TestDedupHoursDefault verifies the default dedup window is 24 hours.
func TestDedupHoursDefault(t *testing.T) {
	store := newLocalFakeSettingsStore()
	if got := DedupHours(store); got != DefaultDedupHours {
		t.Fatalf("DedupHours default = %d, want %d", got, DefaultDedupHours)
	}
	if got := DefaultDedupHours; got != 24 {
		t.Fatalf("DefaultDedupHours = %d, want 24", got)
	}
}

// TestDedupHoursCustom verifies a custom dedup window is loaded correctly.
func TestDedupHoursCustom(t *testing.T) {
	store := newLocalFakeSettingsStore()
	store.Data[SettingsKeyDedupHours] = "48"
	if got := DedupHours(store); got != 48 {
		t.Fatalf("DedupHours with 48 = %d, want 48", got)
	}
}

// TestDedupHoursZero disables dedup when set to 0.
func TestDedupHoursZero(t *testing.T) {
	store := newLocalFakeSettingsStore()
	store.Data[SettingsKeyDedupHours] = "0"
	if got := DedupHours(store); got != 0 {
		t.Fatalf("DedupHours with 0 = %d, want 0", got)
	}
}

// TestDedupHoursInvalid falls back to default for invalid values.
func TestDedupHoursInvalid(t *testing.T) {
	store := newLocalFakeSettingsStore()
	store.Data[SettingsKeyDedupHours] = "not-a-number"
	if got := DedupHours(store); got != DefaultDedupHours {
		t.Fatalf("DedupHours with invalid = %d, want %d", got, DefaultDedupHours)
	}
}

// TestSetDedupHours stores a custom dedup window.
func TestSetDedupHours(t *testing.T) {
	store := newLocalFakeSettingsStore()
	if err := SetDedupHours(store, 12); err != nil {
		t.Fatalf("SetDedupHours returned error: %v", err)
	}
	if got, _ := store.Get(SettingsKeyDedupHours); got != "12" {
		t.Fatalf("stored dedup hours = %q, want 12", got)
	}
}

// TestSetDedupHoursZero allows zero to be set.
func TestSetDedupHoursZero(t *testing.T) {
	store := newLocalFakeSettingsStore()
	if err := SetDedupHours(store, 0); err != nil {
		t.Fatalf("SetDedupHours(0) returned error: %v", err)
	}
	if got, _ := store.Get(SettingsKeyDedupHours); got != "0" {
		t.Fatalf("stored dedup hours = %q, want 0", got)
	}
}

// TestFakeDedupDBRecordAndIsRecentlyServed verifies the fake DB logic.
func TestFakeDedupDBRecordAndIsRecentlyServed(t *testing.T) {
	db := &fakeDedupDB{}
	db.Record(1, "aic", "aic:123")
	if !db.IsRecentlyServed(1, "aic", "aic:123", 24) {
		t.Fatal("expected aic:123 to be recently served within 24h")
	}
	if db.IsRecentlyServed(2, "aic", "aic:123", 24) {
		t.Fatal("expected different device to not match")
	}
	if db.IsRecentlyServed(1, "aic", "aic:999", 24) {
		t.Fatal("expected different artwork to not match")
	}
	if db.IsRecentlyServed(1, "aic", "aic:123", 0) {
		t.Fatal("expected 0 hours to disable dedup")
	}
}

// TestFakeDedupDBCleanup removes old entries.
func TestFakeDedupDBCleanup(t *testing.T) {
	db := &fakeDedupDB{}
	db.Record(1, "aic", "aic:123")
	db.Record(1, "aic", "aic:456")
	db.Cleanup(1, 24*30)
	if len(db.entries) != 2 {
		t.Fatalf("cleanup with large window: got %d entries, want 2", len(db.entries))
	}
	db.Record(1, "aic", "aic:789")
	db.Cleanup(1, 0)
	if len(db.entries) != 3 {
		t.Fatalf("cleanup with 0 hours: got %d entries, want 3", len(db.entries))
	}
}

// TestDedupWithFakeDB verifies service skips recently-served artwork.
func TestDedupWithFakeDB(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 2, 1))
		img.Set(0, 0, color.RGBA{R: 255, A: 255})
		img.Set(1, 0, color.RGBA{B: 255, A: 255})
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, img)
	}))
	defer imageServer.Close()

	provider := &fakeProvider{
		candidates: []Candidate{
			{ID: "a", Title: "A", ImageURL: imageServer.URL},
			{ID: "b", Title: "B", ImageURL: imageServer.URL},
		},
	}
	fakeDB := &fakeDedupDB{}
	store := newLocalFakeSettingsStore()

	svc := NewService(ServiceOptions{
		Provider:   provider,
		HTTPClient: imageServer.Client(),
		Config:     DefaultConfig(),
		Settings:   store,
		HistoryDB:  fakeDB,
	})

	// First fetch should serve "a"
	_, sel1, err := svc.FetchImage(1)
	if err != nil {
		t.Fatalf("first FetchImage(1) error: %v", err)
	}
	if sel1.Candidate.ID != "a" {
		t.Fatalf("first fetch = %q, want a", sel1.Candidate.ID)
	}

	// Second fetch should skip "a" (already served) and serve "b"
	_, sel2, err := svc.FetchImage(1)
	if err != nil {
		t.Fatalf("second FetchImage(1) error: %v", err)
	}
	if sel2.Candidate.ID != "b" {
		t.Fatalf("second fetch = %q, want b (a was deduped)", sel2.Candidate.ID)
	}

	// Third fetch — all candidates deduped — should fall back to "a"
	_, sel3, err := svc.FetchImage(1)
	if err != nil {
		t.Fatalf("third FetchImage(1) error: %v", err)
	}
	if sel3.Candidate.ID != "a" {
		t.Fatalf("third fetch = %q, want a (fallback after all deduped)", sel3.Candidate.ID)
	}
}

// TestGormDedupHistoryDBIntegration tests the GORM implementation with a
// temporary in-memory SQLite database.
func TestGormDedupHistoryDBIntegration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skip("could not open in-memory sqlite:", err)
	}
	if err := db.AutoMigrate(&model.PublicArtServingHistory{}); err != nil {
		t.Skip("could not migrate:", err)
	}

	hdb := NewDedupHistoryDB(db)

	hdb.Record(1, "aic", "aic:100")
	if !hdb.IsRecentlyServed(1, "aic", "aic:100", 24) {
		t.Fatal("expected aic:100 to be recently served")
	}
	if hdb.IsRecentlyServed(1, "aic", "aic:999", 24) {
		t.Fatal("expected aic:999 to not be recently served")
	}

	hdb.Cleanup(1, 24*30)
	if !hdb.IsRecentlyServed(1, "aic", "aic:100", 24) {
		t.Fatal("after generous cleanup, aic:100 should still be present")
	}
}