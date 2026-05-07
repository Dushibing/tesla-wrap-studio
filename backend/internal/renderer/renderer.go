package renderer

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"

	"tesla-wrap-studio/internal/model"
)

// Renderer handles image composition for Tesla wrap templates
type Renderer struct{}

// New creates a new renderer
func New() *Renderer {
	return &Renderer{}
}

// ViewAdjust represents user adjustments for a view
type ViewAdjust struct {
	Scale   int     `json:"scale"`
	Rotate  int     `json:"rotate"`
	OffsetX int     `json:"offsetX"`
	OffsetY int     `json:"offsetY"`
	FlipH   bool    `json:"flipH"`
}

// RenderOptions holds options for rendering
type RenderOptions struct {
	Adjustments map[string]ViewAdjust `json:"adjustments,omitempty"`
}

// Render composites user images onto the vehicle template
func (r *Renderer) Render(m *model.VehicleModel, images map[string]io.Reader, opts ...RenderOptions) (image.Image, error) {
	if m == nil {
		return nil, fmt.Errorf("model is required")
	}

	var options RenderOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	if options.Adjustments == nil {
		options.Adjustments = make(map[string]ViewAdjust)
	}
	templateImg, err := m.TemplateImage()
	if err != nil {
		return nil, err
	}

	// Create output image (RGBA for blending)
	bounds := templateImg.Bounds()
	out := image.NewNRGBA(bounds)

	// Copy template as base (this gives us the black outlines and text)
	draw.Draw(out, bounds, templateImg, bounds.Min, draw.Src)

	// Load and composite each user image onto the corresponding view region
	for _, view := range m.Views {
		reader, ok := images[view.Name]
		if !ok {
			continue
		}

		// Decode user image
		userImg, _, err := image.Decode(reader)
		if err != nil {
			log.Printf("Warning: failed to decode image for view %s: %v", view.Name, err)
			continue
		}

		adj := options.Adjustments[view.Name]

		// Compute final target size once to avoid double scaling quality loss
		scaleFactor := 1.0
		if adj.Scale > 0 {
			scaleFactor = float64(adj.Scale) / 100.0
		}

		srcBounds := userImg.Bounds()
		srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
		if srcW == 0 || srcH == 0 {
			continue
		}

		fitScale := math.Min(float64(view.W)/float64(srcW), float64(view.H)/float64(srcH))
		finalScale := fitScale * scaleFactor
		scaledW := int(float64(srcW) * finalScale)
		scaledH := int(float64(srcH) * finalScale)
		if scaledW <= 0 {
			scaledW = 1
		}
		if scaledH <= 0 {
			scaledH = 1
		}

		var scaled image.Image = scaleNearest(userImg, scaledW, scaledH)

		// Apply horizontal flip
		shouldFlip := view.FlipH
		if adj.FlipH {
			shouldFlip = !shouldFlip
		}
		if shouldFlip {
			scaled = flipHorizontal(scaled)
		}

		// Apply rotation (user + view default)
		rotation := view.Rotation + float64(adj.Rotate)
		if rotation != 0 {
			scaled = rotateImage(scaled, rotation)
		}

		// Center transformed content in the target region, then apply user offsets.
		drawViewImage(out, view, adj.OffsetX, adj.OffsetY, scaled)
	}

	// Apply gap color matching between adjacent views
	r.matchGapColors(out, m)

	// Composite template on top so black outlines and text remain visible
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			tc := templateImg.NRGBAAt(x, y)
			if tc.R < 50 && tc.G < 50 && tc.B < 50 && tc.A > 128 {
				out.SetNRGBA(x, y, tc)
			}
		}
	}

	return out, nil
}

// matchGapColors blends colors across view gaps for seamless appearance
func (r *Renderer) matchGapColors(img *image.NRGBA, m *model.VehicleModel) {
	for _, view := range m.Views {
		if view.GapMatch == "" {
			continue
		}

		// Find the adjacent view by looking at spatial proximity
		var adjacent *model.View
		for i := range m.Views {
			other := &m.Views[i]
			if view.Name == other.Name {
				continue
			}

			switch view.GapMatch {
			case "left", "right":
				// Check if views are vertically overlapping (same row)
				if other.Y < view.Y+view.H && other.Y+other.H > view.Y {
					adjacent = other
				}
			case "top", "bottom":
				// Check if views are horizontally overlapping (same column)
				if other.X < view.X+view.W && other.X+other.W > view.X {
					adjacent = other
				}
			}

			if adjacent != nil {
				break
			}
		}

		if adjacent == nil {
			continue
		}

		switch view.GapMatch {
		case "right":
			blendHorizontalGap(img, view, *adjacent)
		case "left":
			blendHorizontalGap(img, *adjacent, view)
		case "bottom":
			blendVerticalGap(img, view, *adjacent)
		case "top":
			blendVerticalGap(img, *adjacent, view)
		}
	}
}

// blendHorizontalGap blends colors horizontally between two adjacent regions
func blendHorizontalGap(img *image.NRGBA, left, right model.View) {
	gapStart := left.X + left.W
	gapEnd := right.X
	gapWidth := gapEnd - gapStart

	if gapWidth <= 0 || gapWidth > 50 {
		return // No gap or too large
	}

	// Sample edge colors — use int accumulators to avoid uint8 overflow
	var leftR, leftG, leftB int
	var rightR, rightG, rightB int
	leftCount := 0
	rightCount := 0

	// Sample from left edge (rightmost 3 pixels of left region)
	for y := left.Y; y < left.Y+left.H && y < img.Bounds().Max.Y; y++ {
		for x := left.X + left.W - 3; x < left.X+left.W && x < img.Bounds().Max.X; x++ {
			p := img.NRGBAAt(x, y)
			leftR += int(p.R)
			leftG += int(p.G)
			leftB += int(p.B)
			leftCount++
		}
	}

	// Sample from right edge (leftmost 3 pixels of right region)
	for y := right.Y; y < right.Y+right.H && y < img.Bounds().Max.Y; y++ {
		for x := right.X; x < right.X+3 && x < img.Bounds().Max.X; x++ {
			p := img.NRGBAAt(x, y)
			rightR += int(p.R)
			rightG += int(p.G)
			rightB += int(p.B)
			rightCount++
		}
	}

	var leftEdgeColor, rightEdgeColor color.NRGBA
	if leftCount > 0 {
		leftEdgeColor = color.NRGBA{
			R: uint8(leftR / leftCount),
			G: uint8(leftG / leftCount),
			B: uint8(leftB / leftCount),
			A: 255,
		}
	}
	if rightCount > 0 {
		rightEdgeColor = color.NRGBA{
			R: uint8(rightR / rightCount),
			G: uint8(rightG / rightCount),
			B: uint8(rightB / rightCount),
			A: 255,
		}
	}

	// If either edge has no sampled pixels, skip blending
	if leftCount == 0 || rightCount == 0 {
		return
	}

	// Fill the gap with gradient
	overlapY := max(left.Y, right.Y)
	overlapEndY := min(left.Y+left.H, right.Y+right.H)

	for y := overlapY; y < overlapEndY && y < img.Bounds().Max.Y; y++ {
		for x := gapStart; x < gapEnd && x < img.Bounds().Max.X; x++ {
			t := float64(x-gapStart) / float64(gapWidth)
			// Smoothstep for nicer transition
			t = t * t * (3 - 2*t)

			blended := color.NRGBA{
				R: uint8(float64(leftEdgeColor.R)*(1-t) + float64(rightEdgeColor.R)*t),
				G: uint8(float64(leftEdgeColor.G)*(1-t) + float64(rightEdgeColor.G)*t),
				B: uint8(float64(leftEdgeColor.B)*(1-t) + float64(rightEdgeColor.B)*t),
				A: 255,
			}
			img.SetNRGBA(x, y, blended)
		}
	}
}

// blendVerticalGap blends colors vertically between two adjacent regions
func blendVerticalGap(img *image.NRGBA, top, bottom model.View) {
	gapStart := top.Y + top.H
	gapEnd := bottom.Y
	gapHeight := gapEnd - gapStart

	if gapHeight <= 0 || gapHeight > 50 {
		return
	}

	var topR, topG, topB int
	var bottomR, bottomG, bottomB int
	topCount := 0
	bottomCount := 0

	for x := top.X; x < top.X+top.W && x < img.Bounds().Max.X; x++ {
		for y := top.Y + top.H - 3; y < top.Y+top.H && y < img.Bounds().Max.Y; y++ {
			p := img.NRGBAAt(x, y)
			topR += int(p.R)
			topG += int(p.G)
			topB += int(p.B)
			topCount++
		}
	}

	for x := bottom.X; x < bottom.X+bottom.W && x < img.Bounds().Max.X; x++ {
		for y := bottom.Y; y < bottom.Y+3 && y < img.Bounds().Max.Y; y++ {
			p := img.NRGBAAt(x, y)
			bottomR += int(p.R)
			bottomG += int(p.G)
			bottomB += int(p.B)
			bottomCount++
		}
	}

	var topEdgeColor, bottomEdgeColor color.NRGBA
	if topCount > 0 {
		topEdgeColor = color.NRGBA{
			R: uint8(topR / topCount),
			G: uint8(topG / topCount),
			B: uint8(topB / topCount),
			A: 255,
		}
	}
	if bottomCount > 0 {
		bottomEdgeColor = color.NRGBA{
			R: uint8(bottomR / bottomCount),
			G: uint8(bottomG / bottomCount),
			B: uint8(bottomB / bottomCount),
			A: 255,
		}
	}
	if topCount == 0 || bottomCount == 0 {
		return
	}

	overlapX := max(top.X, bottom.X)
	overlapEndX := min(top.X+top.W, bottom.X+bottom.W)

	for x := overlapX; x < overlapEndX && x < img.Bounds().Max.X; x++ {
		for y := gapStart; y < gapEnd && y < img.Bounds().Max.Y; y++ {
			t := float64(y-gapStart) / float64(gapHeight)
			t = t * t * (3 - 2*t)

			blended := color.NRGBA{
				R: uint8(float64(topEdgeColor.R)*(1-t) + float64(bottomEdgeColor.R)*t),
				G: uint8(float64(topEdgeColor.G)*(1-t) + float64(bottomEdgeColor.G)*t),
				B: uint8(float64(topEdgeColor.B)*(1-t) + float64(bottomEdgeColor.B)*t),
				A: 255,
			}
			img.SetNRGBA(x, y, blended)
		}
	}
}

func drawViewImage(dst *image.NRGBA, view model.View, offsetX, offsetY int, src image.Image) {
	targetRect := image.Rect(view.X, view.Y, view.X+view.W, view.Y+view.H)
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW == 0 || srcH == 0 {
		return
	}

	drawRect := image.Rect(
		targetRect.Min.X+(targetRect.Dx()-srcW)/2+offsetX,
		targetRect.Min.Y+(targetRect.Dy()-srcH)/2+offsetY,
		targetRect.Min.X+(targetRect.Dx()-srcW)/2+offsetX+srcW,
		targetRect.Min.Y+(targetRect.Dy()-srcH)/2+offsetY+srcH,
	)

	clipRect := drawRect.Intersect(targetRect).Intersect(dst.Bounds())
	if clipRect.Empty() {
		return
	}

	sourcePoint := srcBounds.Min.Add(clipRect.Min.Sub(drawRect.Min))
	draw.Draw(dst, clipRect, src, sourcePoint, draw.Over)
}

// scaleImageAbsolute scales an image to exact target dimensions
func scaleImageAbsolute(img image.Image, targetW, targetH int) image.Image {
	if targetW <= 0 || targetH <= 0 {
		return img
	}
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return img
	}
	if srcW == targetW && srcH == targetH {
		return convertToNRGBA(img)
	}
	out := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	scaleX := float64(targetW) / float64(srcW)
	scaleY := float64(targetH) / float64(srcH)
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			sx := float64(x) / scaleX
			sy := float64(y) / scaleY
			ix, iy := int(sx), int(sy)
			if ix >= srcW { ix = srcW - 1 }
			if iy >= srcH { iy = srcH - 1 }
			out.SetNRGBA(x, y, toNRGBA(img.At(ix, iy)))
		}
	}
	return out
}

// scaleImageFit scales an image to fit WITHIN target dimensions while maintaining aspect ratio
func scaleImageFit(img image.Image, targetW, targetH int) image.Image {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return img
	}
	if targetW <= 0 || targetH <= 0 {
		return img
	}
	// Compute uniform scale to fit within target
	scale := math.Min(float64(targetW)/float64(srcW), float64(targetH)/float64(srcH))
	scaledW := int(float64(srcW) * scale)
	scaledH := int(float64(srcH) * scale)
	if scaledW == 0 || scaledH == 0 {
		scaledW, scaledH = 1, 1
	}
	return scaleNearest(img, scaledW, scaledH)
}

// scaleImage fills target dimensions (may stretch to fill)
func scaleImage(img image.Image, targetW, targetH int) image.Image {
	return scaleImageFill(img, targetW, targetH)
}

// scaleImageFill scales an image to exactly fill target dimensions (may distort aspect ratio)
func scaleImageFill(img image.Image, targetW, targetH int) image.Image {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	if srcW == 0 || srcH == 0 {
		return img
	}
	if targetW <= 0 || targetH <= 0 {
		return img
	}
	if srcW == targetW && srcH == targetH {
		return convertToNRGBA(img)
	}

	// scaling factor for bilinear sample mapping
	scaleX := float64(targetW) / float64(srcW)
	scaleY := float64(targetH) / float64(srcH)

	// Use bilinear interpolation
	out := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))

	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			// Map output pixel back to source
			sx := float64(x) / scaleX
			sy := float64(y) / scaleY

			sx = math.Max(0, math.Min(float64(srcW-1), sx))
			sy = math.Max(0, math.Min(float64(srcH-1), sy))

			ix, iy := int(sx), int(sy)
			fx, fy := sx-float64(ix), sy-float64(iy)

			if fx == 0 && fy == 0 {
				out.SetNRGBA(x, y, toNRGBA(img.At(ix, iy)))
				continue
			}

			nx := min(ix+1, srcW-1)
			ny := min(iy+1, srcH-1)

			c00 := toNRGBA(img.At(ix, iy))
			c10 := toNRGBA(img.At(nx, iy))
			c01 := toNRGBA(img.At(ix, ny))
			c11 := toNRGBA(img.At(nx, ny))

			blended := color.NRGBA{
				R: uint8(float64(c00.R)*(1-fx)*(1-fy) + float64(c10.R)*fx*(1-fy) + float64(c01.R)*(1-fx)*fy + float64(c11.R)*fx*fy),
				G: uint8(float64(c00.G)*(1-fx)*(1-fy) + float64(c10.G)*fx*(1-fy) + float64(c01.G)*(1-fx)*fy + float64(c11.G)*fx*fy),
				B: uint8(float64(c00.B)*(1-fx)*(1-fy) + float64(c10.B)*fx*(1-fy) + float64(c01.B)*(1-fx)*fy + float64(c11.B)*fx*fy),
				A: uint8(float64(c00.A)*(1-fx)*(1-fy) + float64(c10.A)*fx*(1-fy) + float64(c01.A)*(1-fx)*fy + float64(c11.A)*fx*fy),
			}
			out.SetNRGBA(x, y, blended)
		}
	}

	return out
}

func rotateImage(img image.Image, degrees float64) image.Image {
	if degrees == 0 {
		return img
	}
	// Simple rotation - for production use imaging.Rotate
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	rad := degrees * math.Pi / 180
	cos := math.Cos(rad)
	sin := math.Sin(rad)

	// Calculate new dimensions
	newW := int(math.Abs(float64(w)*cos) + math.Abs(float64(h)*sin))
	newH := int(math.Abs(float64(h)*cos) + math.Abs(float64(w)*sin))
	if newW <= 0 {
		newW = w
	}
	if newH <= 0 {
		newH = h
	}

	out := image.NewNRGBA(image.Rect(0, 0, newW, newH))
	cx, cy := float64(w)/2, float64(h)/2
	ncx, ncy := float64(newW)/2, float64(newH)/2

	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			sx := (float64(x)-ncx)*cos + (float64(y)-ncy)*sin + cx
			sy := -(float64(x)-ncx)*sin + (float64(y)-ncy)*cos + cy

			ix, iy := int(sx), int(sy)
			if ix >= 0 && ix < w && iy >= 0 && iy < h {
				out.SetNRGBA(x, y, toNRGBA(img.At(ix, iy)))
			}
		}
	}

	return out
}

func flipHorizontal(img image.Image) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	out := image.NewNRGBA(bounds)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(w-1-x, y, toNRGBA(img.At(x, y)))
		}
	}
	return out
}

func toNRGBA(c color.Color) color.NRGBA {
	if nc, ok := c.(color.NRGBA); ok {
		return nc
	}
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

func convertToNRGBA(img image.Image) *image.NRGBA {
	if out, ok := img.(*image.NRGBA); ok {
		return out
	}
	bounds := img.Bounds()
	out := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			out.SetNRGBA(x, y, toNRGBA(img.At(x, y)))
		}
	}
	return out
}

// SavePNG saves a PNG image to file
func SavePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// scaleNearest nearest-neighbor scaling — fast, maintains pixel crispness
func scaleNearest(img image.Image, targetW, targetH int) *image.NRGBA {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return convertToNRGBA(img)
	}

	out := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	scaleX := float64(srcW) / float64(targetW)
	scaleY := float64(srcH) / float64(targetH)

	for y := 0; y < targetH; y++ {
		sy := int(float64(y) * scaleY)
		if sy >= srcH {
			sy = srcH - 1
		}
		for x := 0; x < targetW; x++ {
			sx := int(float64(x) * scaleX)
			if sx >= srcW {
				sx = srcW - 1
			}
			out.SetNRGBA(x, y, toNRGBA(img.At(sx, sy)))
		}
	}
	return out
}
