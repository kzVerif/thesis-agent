package service

import (
	"image"
	"testing"
)

func TestFitImagePreservesAspectRatio(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		wantWidth  int
		wantHeight int
	}{
		{"full HD", 1920, 1080, 1280, 720},
		{"portrait", 1080, 1920, 405, 720},
		{"small image", 800, 600, 800, 600},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := fitImage(image.NewRGBA(image.Rect(0, 0, test.width, test.height)), 1280, 720).Bounds()
			if got.Dx() != test.wantWidth || got.Dy() != test.wantHeight {
				t.Fatalf("got %dx%d, want %dx%d", got.Dx(), got.Dy(), test.wantWidth, test.wantHeight)
			}
		})
	}
}
