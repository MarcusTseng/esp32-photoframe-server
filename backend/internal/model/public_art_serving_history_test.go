package model

import "testing"

func TestPublicArtServingHistoryTableNameMatchesMigration(t *testing.T) {
	got := PublicArtServingHistory{}.TableName()
	want := "public_art_serving_history"
	if got != want {
		t.Fatalf("TableName() = %q, want %q", got, want)
	}
}
