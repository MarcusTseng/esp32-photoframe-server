package handler

import "testing"

func TestImageURLWithTokenAddsToken(t *testing.T) {
	got := imageURLWithToken("http://homeassistant.local:9607/image/public_art", "abc123")
	want := "http://homeassistant.local:9607/image/public_art?token=abc123"
	if got != want {
		t.Fatalf("imageURLWithToken() = %q, want %q", got, want)
	}
}

func TestImageURLWithTokenPreservesExistingQuery(t *testing.T) {
	got := imageURLWithToken("http://homeassistant.local:9607/image/public_art?foo=bar", "abc 123")
	want := "http://homeassistant.local:9607/image/public_art?foo=bar&token=abc+123"
	if got != want {
		t.Fatalf("imageURLWithToken() = %q, want %q", got, want)
	}
}

func TestImageURLWithTokenReplacesOldToken(t *testing.T) {
	got := imageURLWithToken("http://homeassistant.local:9607/image/public_art?token=old&foo=bar", "new")
	want := "http://homeassistant.local:9607/image/public_art?foo=bar&token=new"
	if got != want {
		t.Fatalf("imageURLWithToken() = %q, want %q", got, want)
	}
}

func TestImageURLWithTokenLeavesEmptyInputsUnchanged(t *testing.T) {
	if got := imageURLWithToken("http://homeassistant.local:9607/image/public_art", ""); got != "http://homeassistant.local:9607/image/public_art" {
		t.Fatalf("imageURLWithToken() with empty token = %q", got)
	}
	if got := imageURLWithToken("", "abc123"); got != "" {
		t.Fatalf("imageURLWithToken() with empty URL = %q", got)
	}
}
