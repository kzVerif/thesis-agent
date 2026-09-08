package service

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/kbinani/screenshot"
	"golang.org/x/image/draw"
)

const (
	ScreenMaxWidth  = 1280
	ScreenMaxHeight = 720
	ScreenQuality   = 60
)

// CaptureScreenJPEG captures the primary display and returns a JPEG that fits
// inside 1280x720 without changing its aspect ratio.
func CaptureScreenJPEG() ([]byte, error) {
	if screenshot.NumActiveDisplays() == 0 {
		return nil, fmt.Errorf("no active display")
	}

	source, err := screenshot.CaptureRect(screenshot.GetDisplayBounds(0))
	if err != nil {
		return nil, fmt.Errorf("capture primary display: %w", err)
	}

	frame := fitImage(source, ScreenMaxWidth, ScreenMaxHeight)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, frame, &jpeg.Options{Quality: ScreenQuality}); err != nil {
		return nil, fmt.Errorf("encode screen as JPEG: %w", err)
	}
	return encoded.Bytes(), nil
}

func fitImage(source image.Image, maxWidth, maxHeight int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxWidth && height <= maxHeight {
		return source
	}

	scale := min(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height))
	target := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale))))
	draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)
	return target
}
