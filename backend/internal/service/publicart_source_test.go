package service

import (
	"image"
	"image/color"
	"testing"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/publicart"
)

type fakePublicArtFetcher struct{}

func (fakePublicArtFetcher) FetchImage() (image.Image, publicart.SelectedArtwork, error) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	return img, publicart.SelectedArtwork{Candidate: publicart.Candidate{ID: "aic:1"}}, nil
}

func (fakePublicArtFetcher) FetchImageWithComposition(targetW, targetH int) (image.Image, publicart.SelectedArtwork, error) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	return img, publicart.SelectedArtwork{Candidate: publicart.Candidate{ID: "aic:1"}}, nil
}

func TestPublicArtSourceNameAndFetch(t *testing.T) {
	source := NewPublicArtSource(fakePublicArtFetcher{})
	if source.Name() != model.SourcePublicArt {
		t.Fatalf("Name() = %q, want %q", source.Name(), model.SourcePublicArt)
	}

	resp, err := source.Fetch(&imagesource.Request{Width: 2, Height: 2})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if resp.Image == nil {
		t.Fatal("Fetch returned nil image")
	}
	if resp.SkipPostProcessing {
		t.Fatal("Public art should use normal photo post-processing")
	}
}
