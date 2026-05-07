package renderer

import (
	"image"
	"image/color"
	"testing"
	"tesla-wrap-studio/internal/model"
)

func TestScaleImageFit(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100)) // 2:1 aspect ratio
	
	// Should scale to 100x50 to fit 100x100
	scaled := scaleImageFit(img, 100, 100)
	if scaled.Bounds().Dx() != 100 || scaled.Bounds().Dy() != 50 {
		t.Errorf("expected 100x50, got %dx%d", scaled.Bounds().Dx(), scaled.Bounds().Dy())
	}

	// Should scale to 50x25 to fit 50x50
	scaled = scaleImageFit(img, 50, 50)
	if scaled.Bounds().Dx() != 50 || scaled.Bounds().Dy() != 25 {
		t.Errorf("expected 50x25, got %dx%d", scaled.Bounds().Dx(), scaled.Bounds().Dy())
	}
}

func TestScaleImageAbsolute(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	scaled := scaleImageAbsolute(img, 20, 30)
	if scaled.Bounds().Dx() != 20 || scaled.Bounds().Dy() != 30 {
		t.Errorf("expected 20x30, got %dx%d", scaled.Bounds().Dx(), scaled.Bounds().Dy())
	}
}

func TestScaleNearest(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(1, 1, color.NRGBA{0, 255, 0, 255})

	scaled := scaleNearest(img, 4, 4)
	if scaled.Bounds().Dx() != 4 || scaled.Bounds().Dy() != 4 {
		t.Errorf("expected 4x4, got %dx%d", scaled.Bounds().Dx(), scaled.Bounds().Dy())
	}

	// Top-left should be red
	c := scaled.NRGBAAt(0, 0)
	if c.R != 255 || c.G != 0 {
		t.Errorf("expected red at 0,0, got %+v", c)
	}
	// Bottom-right should be green
	c = scaled.NRGBAAt(3, 3)
	if c.R != 0 || c.G != 255 {
		t.Errorf("expected green at 3,3, got %+v", c)
	}

	// Edge case: single pixel
	img1 := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img1.SetNRGBA(0, 0, color.NRGBA{0, 0, 255, 255})
	scaled1 := scaleNearest(img1, 10, 10)
	c1 := scaled1.NRGBAAt(5, 5)
	if c1.B != 255 {
		t.Errorf("expected blue, got %+v", c1)
	}
}

func TestToNRGBA(t *testing.T) {
	c := color.RGBA{255, 0, 0, 255}
	nc := toNRGBA(c)
	if nc.R != 255 || nc.A != 255 {
		t.Errorf("expected NRGBA 255,0,0,255, got %+v", nc)
	}

	// Test premultiplied alpha
	c2 := color.RGBA{128, 0, 0, 128} // Premultiplied
	nc2 := toNRGBA(c2)
	if nc2.R != 255 { // 128/128 * 255 = 255
		t.Errorf("expected NRGBA 255 at 128 alpha, got %d", nc2.R)
	}
}

func TestBlendHorizontalGap(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	
	left := model.View{X: 10, Y: 10, W: 20, H: 20}
	right := model.View{X: 35, Y: 10, W: 20, H: 20} // Gap of 5 pixels
	
	// Fill regions with solid colors
	for y := 10; y < 30; y++ {
		for x := 10; x < 30; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
		}
		for x := 35; x < 55; x++ {
			img.SetNRGBA(x, y, color.NRGBA{0, 0, 255, 255})
		}
	}

	blendHorizontalGap(img, left, right)

	// Check middle of gap (x=32)
	mid := img.NRGBAAt(32, 20)
	if mid.R == 0 && mid.B == 0 {
		t.Errorf("gap not filled at x=32, got %+v", mid)
	}
	if mid.R > 0 && mid.B > 0 {
		// Blend should be roughly purple
	} else {
		t.Errorf("unexpected color at x=32: %+v", mid)
	}
}

func TestBlendVerticalGap(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	top := model.View{X: 10, Y: 10, W: 20, H: 20}
	bottom := model.View{X: 10, Y: 35, W: 20, H: 20} // Gap of 5 pixels
	
	for x := 10; x < 30; x++ {
		for y := 10; y < 30; y++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
		}
		for y := 35; y < 55; y++ {
			img.SetNRGBA(x, y, color.NRGBA{0, 0, 255, 255})
		}
	}

	blendVerticalGap(img, top, bottom)

	mid := img.NRGBAAt(20, 32)
	if mid.R == 0 && mid.B == 0 {
		t.Errorf("gap not filled at y=32, got %+v", mid)
	}
}

func TestDrawViewImage(t *testing.T) {
	dst := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	view := model.View{X: 10, Y: 10, W: 20, H: 20}
	src := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			src.SetNRGBA(x, y, color.NRGBA{0, 255, 0, 255})
		}
	}

	// Draw centered with 0 offset
	drawViewImage(dst, view, 0, 0, src)
	
	// src is 10x10, view is 20x20. Center is (10+10/2, 10+10/2) = (15, 15)
	// src should be at (15, 15) to (25, 25)
	// Wait, center logic: targetRect.Min.X + (targetRect.Dx()-srcW)/2 = 10 + (20-10)/2 = 15.
	// So it should be at (15, 15) to (25, 25).
	c := dst.NRGBAAt(15, 15)
	if c.G != 255 {
		t.Errorf("expected green at 15,15, got %+v", c)
	}
	c = dst.NRGBAAt(9, 9)
	if c.G != 0 {
		t.Errorf("should not be green at 9,9, got %+v", c)
	}
}

func TestFlipHorizontal(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	
	flipped := flipHorizontal(img).(*image.NRGBA)
	if flipped.NRGBAAt(1, 0).R != 255 {
		t.Errorf("expected red at 1,0 after flip, got %+v", flipped.NRGBAAt(1, 0))
	}
}

func TestRotateImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 2))
	// Fill top half red
	for x := 0; x < 10; x++ {
		img.SetNRGBA(x, 0, color.NRGBA{255, 0, 0, 255})
	}
	
	// Rotate 90 degrees
	rotated := rotateImage(img, 90)
	// New bounds should be roughly 2x10
	if rotated.Bounds().Dx() < 1 || rotated.Bounds().Dy() < 9 {
		t.Errorf("unexpected bounds after rotation: %v", rotated.Bounds())
	}
}

func TestConvertToNRGBA(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	out := convertToNRGBA(img)
	if out == nil {
		t.Fatal("expected NRGBA image, got nil")
	}
}
