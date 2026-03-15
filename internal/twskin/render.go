package twskin

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"math"

	"golang.org/x/image/draw"
)

// Base Teeworlds 0.6 skin resolution.
const (
	baseSkinW = 256
	baseSkinH = 128
	gridCols  = 8
	gridRows  = 4
	cellW     = baseSkinW / gridCols // 32
	cellH     = baseSkinH / gridRows // 32
)

// Sprite rectangles within the 256x128 base skin.
// Grid positions taken from DDNet datasrc/content.py (set_tee: gridx=8, gridy=4).
//
//	Sprite("tee_body",         set_tee, 0, 0, 3, 3)
//	Sprite("tee_body_outline", set_tee, 3, 0, 3, 3)
//	Sprite("tee_foot",         set_tee, 6, 1, 2, 1)
//	Sprite("tee_foot_outline", set_tee, 6, 2, 2, 1)
//	Sprite("tee_eye_normal",   set_tee, 2, 3, 1, 1)
var (
	sprBody        = grc(0, 0, 3, 3)
	sprBodyOutline = grc(3, 0, 3, 3)
	sprFoot        = grc(6, 1, 2, 1)
	sprFootOutline = grc(6, 2, 2, 1)
	sprEyeNormal   = grc(2, 3, 1, 1)
)

// grc converts grid coordinates to pixel rectangle.
func grc(gx, gy, gw, gh int) image.Rectangle {
	return image.Rect(gx*cellW, gy*cellH, (gx+gw)*cellW, (gy+gh)*cellH)
}

// Rendering constants derived from DDNet RenderTee6 at BaseSize = 64.
//
// Idle animation keyframes (from DDNet datasrc/content.py):
//
//	ANIM_BASE:  body (0,-4), back_foot (0,10), front_foot (0,10)
//	ANIM_IDLE:  back_foot (-7,0), front_foot (7,0)
//	Combined:   body (0,-4), back_foot (-7,10), front_foot (7,10)
//
// Body quad:  BaseSize x BaseSize   = 64x64, centred at BodyPos.
// Foot quad:  BaseSize x BaseSize/2 = 64x32, centred at foot pos.
// Eye sprite: BaseSize * 0.40       = 25.6 per eye.
//
// Eye offset for Direction (1, 0):
//
//	EyeScale      = BaseSize * 0.40                      = 25.6
//	EyeSeparation = (0.075 - 0.010*|Dir.x|) * BaseSize   = 4.16
//	Offset.x      = Dir.x * 0.125 * BaseSize             = 8.0
//	Offset.y      = (-0.05 + Dir.y*0.10) * BaseSize      = -3.2
const (
	baseSize = 64.0

	bodyX = 0.0
	bodyY = -4.0

	backFootX  = -7.0
	backFootY  = 10.0
	frontFootX = 7.0
	frontFootY = 10.0

	bodyDisplayW = baseSize
	bodyDisplayH = baseSize
	footDisplayW = baseSize
	footDisplayH = baseSize / 2.0

	eyeScale = baseSize * 0.40

	eyeSep  = 4.16
	eyeOffX = 8.0
	eyeOffY = -3.2
)

// RenderIdleTee composites a front-facing idle Tee with default (normal) eyes
// from a TW 0.6 / DDNet skin PNG. HD skins are first scaled down to the base
// 256x128 resolution so the output matches the classic 0.6 sprite dimensions.
// The result is a tightly-cropped NRGBA image with a transparent background.
func RenderIdleTee(r io.Reader) (image.Image, error) {
	src, err := png.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode skin png: %w", err)
	}

	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return nil, fmt.Errorf("skin image has zero dimensions")
	}
	if srcW*baseSkinH != srcH*baseSkinW {
		return nil, fmt.Errorf("skin aspect ratio must be %d:%d, got %dx%d",
			baseSkinW, baseSkinH, srcW, srcH)
	}

	skin := scaleToBase(src)

	body := skin.SubImage(sprBody).(*image.NRGBA)
	bodyOut := skin.SubImage(sprBodyOutline).(*image.NRGBA)
	foot := skin.SubImage(sprFoot).(*image.NRGBA)
	footOut := skin.SubImage(sprFootOutline).(*image.NRGBA)
	eye := skin.SubImage(sprEyeNormal).(*image.NRGBA)

	// Compute tight bounding box of the composed Tee.
	type part struct{ cx, cy, hw, hh float64 }
	parts := [...]part{
		{bodyX, bodyY, bodyDisplayW / 2, bodyDisplayH / 2},
		{backFootX, backFootY, footDisplayW / 2, footDisplayH / 2},
		{frontFootX, frontFootY, footDisplayW / 2, footDisplayH / 2},
	}
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	for _, p := range parts {
		minX = min(minX, p.cx-p.hw)
		minY = min(minY, p.cy-p.hh)
		maxX = max(maxX, p.cx+p.hw)
		maxY = max(maxY, p.cy+p.hh)
	}

	canvasW := int(math.Ceil(maxX - minX))
	canvasH := int(math.Ceil(maxY - minY))
	canvas := image.NewNRGBA(image.Rect(0, 0, canvasW, canvasH))

	// Map game-coordinate origin to canvas pixel offset.
	ox, oy := -minX, -minY

	// drawCentred scales sprite into a (dw x dh) rect centred at (cx, cy).
	drawCentred := func(sprite image.Image, cx, cy, dw, dh float64) {
		dx := ox + cx - dw/2
		dy := oy + cy - dh/2
		dstRect := image.Rect(
			int(math.Round(dx)),
			int(math.Round(dy)),
			int(math.Round(dx+dw)),
			int(math.Round(dy+dh)),
		)
		draw.CatmullRom.Scale(canvas, dstRect, sprite, sprite.Bounds(), draw.Over, nil)
	}

	// Compositing order from DDNet RenderTee6 (back to front):
	//
	// Pass 0 (outline):
	//   Filling 0 -> back foot outline
	//   Filling 1 -> body outline, front foot outline
	// Pass 1 (fill):
	//   Filling 0 -> back foot fill
	//   Filling 1 -> body fill, eyes (normal), front foot fill

	drawCentred(footOut, backFootX, backFootY, footDisplayW, footDisplayH)
	drawCentred(bodyOut, bodyX, bodyY, bodyDisplayW, bodyDisplayH)
	drawCentred(footOut, frontFootX, frontFootY, footDisplayW, footDisplayH)

	drawCentred(foot, backFootX, backFootY, footDisplayW, footDisplayH)
	drawCentred(body, bodyX, bodyY, bodyDisplayW, bodyDisplayH)

	// Eyes: left is drawn normally, right is horizontally mirrored.
	leftEyeCX := bodyX + eyeOffX - eyeSep
	leftEyeCY := bodyY + eyeOffY
	rightEyeCX := bodyX + eyeOffX + eyeSep
	rightEyeCY := bodyY + eyeOffY

	drawCentred(eye, leftEyeCX, leftEyeCY, eyeScale, eyeScale)
	drawCentred(mirrorH(eye), rightEyeCX, rightEyeCY, eyeScale, eyeScale)

	drawCentred(foot, frontFootX, frontFootY, footDisplayW, footDisplayH)

	return canvas, nil
}

// scaleToBase scales src to 256x128. If already at base resolution the pixels
// are simply copied into an *image.NRGBA.
func scaleToBase(src image.Image) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, baseSkinW, baseSkinH))
	bounds := src.Bounds()
	if bounds.Dx() == baseSkinW && bounds.Dy() == baseSkinH {
		draw.Copy(dst, image.Point{}, src, bounds, draw.Src, nil)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)
	}
	return dst
}

// mirrorH returns a horizontally mirrored copy of img.
func mirrorH(img image.Image) image.Image {
	b := img.Bounds()
	w := b.Dx()
	dst := image.NewNRGBA(image.Rect(0, 0, w, b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(w-1-(x-b.Min.X), y-b.Min.Y, img.At(x, y))
		}
	}
	return dst
}
