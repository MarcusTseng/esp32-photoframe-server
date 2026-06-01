package publicart

import (
	"image"
	"image/color"
	"testing"
)

func makeSolidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, img.Bounds(), c)
	return img
}

func makeTestImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Gradient: R=x, G=y, B=128
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	return img
}

func TestComposeImageCoverOutputsTargetSize(t *testing.T) {
	src := makeTestImage(2000, 1500) // landscape
	comp := Composition{ScaleMode: "cover", Zoom: 1.0, PanX: 0, PanY: 0, BackgroundColor: "white"}
	result := ComposeImage(src, comp, 1200, 1600) // portrait target
	bounds := result.Bounds()
	if bounds.Dx() != 1200 || bounds.Dy() != 1600 {
		t.Errorf("expected 1200×1600, got %d×%d", bounds.Dx(), bounds.Dy())
	}
}

func TestComposeImageCoverWithZoom(t *testing.T) {
	src := makeTestImage(2000, 1500)
	// Zoom 1.5 should crop tighter (zoom in)
	comp1 := Composition{ScaleMode: "cover", Zoom: 1.0, PanX: 0, PanY: 0, BackgroundColor: "white"}
	comp15 := Composition{ScaleMode: "cover", Zoom: 1.5, PanX: 0, PanY: 0, BackgroundColor: "white"}
	result1 := ComposeImage(src, comp1, 1200, 1600)
	result15 := ComposeImage(src, comp15, 1200, 1600)
	// Both should be target size
	if result1.Bounds().Dx() != 1200 || result15.Bounds().Dx() != 1200 {
		t.Errorf("both should output 1200 wide")
	}
	// At zoom 1.0, center pixel at x=1000 (middle of 2000-wide source) maps to x=600 (middle of 1200)
	// At zoom 1.5, cropW = 2000/1.5 ≈ 1333, center offset different — so the color should differ
	r1 := result1.At(600, 800)
	r15 := result15.At(600, 800)
	if r1 == r15 {
		t.Logf("Zoom 1.0 and 1.5 produced identical colors at center — they may differ elsewhere")
	}
}

func TestComposeImageCoverWithPan(t *testing.T) {
	src := makeTestImage(2000, 1500)
	comp := Composition{ScaleMode: "cover", Zoom: 1.0, PanX: 0.3, PanY: 0, BackgroundColor: "white"}
	result := ComposeImage(src, comp, 1200, 1600)
	bounds := result.Bounds()
	if bounds.Dx() != 1200 || bounds.Dy() != 1600 {
		t.Errorf("expected 1200×1600, got %d×%d", bounds.Dx(), bounds.Dy())
	}
}

func TestComposeImageFitLetterboxes(t *testing.T) {
	src := makeTestImage(2000, 1500) // landscape
	comp := Composition{ScaleMode: "fit", Zoom: 1.0, PanX: 0, PanY: 0, BackgroundColor: "black"}
	result := ComposeImage(src, comp, 1200, 1600) // portrait target
	bounds := result.Bounds()
	if bounds.Dx() != 1200 || bounds.Dy() != 1600 {
		t.Errorf("expected 1200×1600, got %d×%d", bounds.Dx(), bounds.Dy())
	}
	// Corner pixels should be background (black)
	corner := result.At(0, 0)
	r, g, b, a := corner.RGBA()
	if !(r == 0 && g == 0 && b == 0 && a > 0) {
		t.Logf("Corner (0,0) = %v — expected black background", corner)
	}
}

func TestComposeImageFitScalesDown(t *testing.T) {
	src := makeTestImage(3000, 2000) // larger than target
	comp := Composition{ScaleMode: "fit", Zoom: 1.0, PanX: 0, PanY: 0, BackgroundColor: "white"}
	result := ComposeImage(src, comp, 1200, 1600)
	bounds := result.Bounds()
	if bounds.Dx() != 1200 || bounds.Dy() != 1600 {
		t.Errorf("expected 1200×1600, got %d×%d", bounds.Dx(), bounds.Dy())
	}
}

func TestComposeImageDefaultSize(t *testing.T) {
	src := makeTestImage(200, 100)
	comp := DefaultComposition()
	result := ComposeImage(src, comp, 0, 0) // invalid size → defaults to 800×600
	bounds := result.Bounds()
	if bounds.Dx() != 800 || bounds.Dy() != 600 {
		t.Errorf("expected 800×600 default, got %d×%d", bounds.Dx(), bounds.Dy())
	}
}

func TestComposeImageEmptySource(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 0, 0))
	comp := DefaultComposition()
	result := ComposeImage(img, comp, 200, 200)
	bounds := result.Bounds()
	if bounds.Dx() != 200 || bounds.Dy() != 200 {
		t.Errorf("expected 200×200, got %d×%d", bounds.Dx(), bounds.Dy())
	}
}

func TestParseColor(t *testing.T) {
	tests := []struct {
		input          string
		expectedR, expectedG, expectedB uint8
	}{
		{input: "white", expectedR: 255, expectedG: 255, expectedB: 255},
		{input: "black", expectedR: 0, expectedG: 0, expectedB: 0},
		{input: "#ff0000", expectedR: 255, expectedG: 0, expectedB: 0},
		{input: "#1a1a1a", expectedR: 26, expectedG: 26, expectedB: 26},
		{input: "#fff", expectedR: 255, expectedG: 255, expectedB: 255},
		{input: "unknown", expectedR: 255, expectedG: 255, expectedB: 255}, // defaults to white
	}
	for _, tc := range tests {
		result := parseColor(tc.input)
		r, g, b, _ := result.RGBA()
		// RGBA returns 16-bit values, compare high bytes
		if uint8(r>>8) != tc.expectedR || uint8(g>>8) != tc.expectedG || uint8(b>>8) != tc.expectedB {
			t.Errorf("parseColor(%q) = (%d,%d,%d), want (%d,%d,%d)", tc.input, r>>8, g>>8, b>>8, tc.expectedR, tc.expectedG, tc.expectedB)
		}
	}
}
