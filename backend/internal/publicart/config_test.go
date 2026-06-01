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

func TestSelectedCandidateSettingsHelpers(t *testing.T) {
	store := &fakeSettingsStore{}
	if _, ok, err := LoadSelectedCandidate(store); err != nil || ok {
		t.Fatalf("LoadSelectedCandidate empty = ok %v err %v, want false nil", ok, err)
	}

	candidate := Candidate{Provider: ProviderAIC, ID: "aic:1", Title: "Water Lilies", ImageURL: "https://example.test/image.jpg"}
	if err := SaveSelectedCandidate(store, candidate); err != nil {
		t.Fatalf("SaveSelectedCandidate returned error: %v", err)
	}
	loaded, ok, err := LoadSelectedCandidate(store)
	if err != nil {
		t.Fatalf("LoadSelectedCandidate returned error: %v", err)
	}
	if !ok || loaded.ID != candidate.ID || loaded.ImageURL != candidate.ImageURL {
		t.Fatalf("loaded = %#v ok=%v, want %#v true", loaded, ok, candidate)
	}
	if err := ClearSelectedCandidate(store); err != nil {
		t.Fatalf("ClearSelectedCandidate returned error: %v", err)
	}
	if got := store.values[SettingsKeySelectedCandidate]; got != "" {
		t.Fatalf("cleared value = %q, want empty", got)
	}
}

func TestSaveSelectedCandidateRejectsMissingImageURL(t *testing.T) {
	store := &fakeSettingsStore{}
	if err := SaveSelectedCandidate(store, Candidate{Provider: ProviderAIC, ID: "aic:1"}); err == nil {
		t.Fatal("SaveSelectedCandidate returned nil error for missing image_url")
	}
}
