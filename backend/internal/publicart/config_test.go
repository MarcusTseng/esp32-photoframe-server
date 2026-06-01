package publicart

import "testing"

type fakeSettingsGetter struct {
	value string
	err   error
}

func (g fakeSettingsGetter) Get(key string) (string, error) {
	return g.value, g.err
}

type fakeSettingsStore struct {
	values map[string]string
}

func (s *fakeSettingsStore) Get(key string) (string, error) {
	if s.values == nil {
		return "", nil
	}
	return s.values[key], nil
}

func (s *fakeSettingsStore) Set(key string, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Query != "art" {
		t.Fatalf("Query = %q, want art", cfg.Query)
	}
	if cfg.Provider != ProviderAIC {
		t.Fatalf("Provider = %q, want %q", cfg.Provider, ProviderAIC)
	}
	if cfg.MinImageLongEdge != 1600 {
		t.Fatalf("MinImageLongEdge = %d, want 1600", cfg.MinImageLongEdge)
	}
	if cfg.PreferredImageLongEdge != 2000 {
		t.Fatalf("PreferredImageLongEdge = %d, want 2000", cfg.PreferredImageLongEdge)
	}
}

func TestSettingsConfigProviderMergesStoredJSONWithDefaults(t *testing.T) {
	provider := NewSettingsConfigProvider(fakeSettingsGetter{
		value: `{"query":"monet","min_image_long_edge":1800}`,
	})

	cfg, err := provider.PublicArtConfig()
	if err != nil {
		t.Fatalf("PublicArtConfig returned error: %v", err)
	}
	if cfg.Query != "monet" {
		t.Fatalf("Query = %q, want monet", cfg.Query)
	}
	if cfg.Provider != ProviderAIC {
		t.Fatalf("Provider = %q, want default %q", cfg.Provider, ProviderAIC)
	}
	if cfg.MinImageLongEdge != 1800 {
		t.Fatalf("MinImageLongEdge = %d, want 1800", cfg.MinImageLongEdge)
	}
	if cfg.PreferredImageLongEdge != 2000 {
		t.Fatalf("PreferredImageLongEdge = %d, want default 2000", cfg.PreferredImageLongEdge)
	}
}

func TestSettingsConfigProviderDefaultsWhenSettingMissing(t *testing.T) {
	provider := NewSettingsConfigProvider(fakeSettingsGetter{})

	cfg, err := provider.PublicArtConfig()
	if err != nil {
		t.Fatalf("PublicArtConfig returned error: %v", err)
	}
	if cfg != DefaultConfig() {
		t.Fatalf("PublicArtConfig = %#v, want default %#v", cfg, DefaultConfig())
	}
}

func TestRankCandidatesPrefersResolutionThenTitle(t *testing.T) {
	candidates := []Candidate{
		{ID: "low", Title: "Zebra", Width: 800, Height: 600},
		{ID: "preferred", Title: "Apple", Width: 3000, Height: 2000},
		{ID: "minimum", Title: "Mango", Width: 1600, Height: 900},
	}

	ranked := RankCandidates(candidates, DefaultConfig())
	if ranked[0].ID != "preferred" {
		t.Fatalf("top candidate = %q, want preferred; ranked=%#v", ranked[0].ID, ranked)
	}
	if ranked[1].ID != "minimum" || ranked[2].ID != "low" {
		t.Fatalf("unexpected ranking order: %#v", ranked)
	}
	if candidates[0].ID != "low" {
		t.Fatalf("RankCandidates mutated input slice")
	}
}

func TestRankCandidatesPrefersRequestedOrientation(t *testing.T) {
	candidates := []Candidate{
		{ID: "portrait", Title: "Portrait", ImageURL: "https://example.test/p.jpg", Width: 1200, Height: 2000},
		{ID: "landscape", Title: "Landscape", ImageURL: "https://example.test/l.jpg", Width: 2000, Height: 1200},
	}

	ranked := RankCandidates(candidates, Config{Orientation: "portrait", MinImageLongEdge: 1000, PreferredImageLongEdge: 1000})
	if ranked[0].ID != "portrait" {
		t.Fatalf("top portrait-ranked candidate = %q, want portrait; ranked=%#v", ranked[0].ID, ranked)
	}

	ranked = RankCandidates(candidates, Config{Orientation: "landscape", MinImageLongEdge: 1000, PreferredImageLongEdge: 1000})
	if ranked[0].ID != "landscape" {
		t.Fatalf("top landscape-ranked candidate = %q, want landscape; ranked=%#v", ranked[0].ID, ranked)
	}
}

func TestSelectedArtworkSettingsHelpers(t *testing.T) {
	store := &fakeSettingsStore{}
	if _, ok, err := LoadSelectedArtwork(store); err != nil || ok {
		t.Fatalf("LoadSelectedArtwork empty = ok %v err %v, want false nil", ok, err)
	}

	candidate := Candidate{Provider: ProviderAIC, ID: "aic:1", Title: "Water Lilies", ImageURL: "https://example.test/image.jpg"}
	artwork := SelectedArtwork{Candidate: candidate, Composition: DefaultComposition()}
	if err := SaveSelectedArtwork(store, artwork); err != nil {
		t.Fatalf("SaveSelectedArtwork returned error: %v", err)
	}
	loaded, ok, err := LoadSelectedArtwork(store)
	if err != nil {
		t.Fatalf("LoadSelectedArtwork returned error: %v", err)
	}
	if !ok || loaded.Candidate.ID != candidate.ID || loaded.Candidate.ImageURL != candidate.ImageURL {
		t.Fatalf("loaded = %#v ok=%v, want %#v true", loaded, ok, artwork)
	}
	if err := ClearSelectedArtwork(store); err != nil {
		t.Fatalf("ClearSelectedArtwork returned error: %v", err)
	}
	if got := store.values[SettingsKeySelectedArtwork]; got != "" {
		t.Fatalf("cleared value = %q, want empty", got)
	}
}

func TestSaveSelectedArtworkRejectsMissingImageURL(t *testing.T) {
	store := &fakeSettingsStore{}
	if err := SaveSelectedArtwork(store, SelectedArtwork{Candidate: Candidate{Provider: ProviderAIC, ID: "aic:1"}}); err == nil {
		t.Fatal("SaveSelectedArtwork returned nil error for missing image_url")
	}
}

func TestSaveSelectedArtworkRejectsInvalidScaleMode(t *testing.T) {
	store := &fakeSettingsStore{}
	if err := SaveSelectedArtwork(store, SelectedArtwork{
		Candidate:   Candidate{Provider: ProviderAIC, ID: "aic:1", ImageURL: "https://example.test/img.jpg"},
		Composition: Composition{ScaleMode: "invalid"},
	}); err == nil {
		t.Fatal("SaveSelectedArtwork returned nil error for invalid scale_mode")
	}
}

func TestLoadSelectedArtworkBackwardCompatWithCandidate(t *testing.T) {
	// Old stored value: plain Candidate (no composition)
	store := &fakeSettingsStore{
		values: map[string]string{
			SettingsKeySelectedArtwork: `{"provider":"aic","id":"aic:1","title":"Water Lilies","image_url":"https://example.test/img.jpg"}`,
		},
	}
	loaded, ok, err := LoadSelectedArtwork(store)
	if err != nil {
		t.Fatalf("LoadSelectedArtwork returned error: %v", err)
	}
	if !ok {
		t.Fatal("LoadSelectedArtwork returned ok=false, want true")
	}
	if loaded.Candidate.ID != "aic:1" {
		t.Fatalf("loaded.Candidate.ID = %q, want aic:1", loaded.Candidate.ID)
	}
	if loaded.Composition.ScaleMode != "cover" {
		t.Fatalf("loaded.Composition.ScaleMode = %q, want cover (default)", loaded.Composition.ScaleMode)
	}
}
