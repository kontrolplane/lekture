package image

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	"github.com/disintegration/imaging"
)

// Render loads an image from path and renders it as pixel-art using ANSI
// colored half-block characters (▀). Each character cell encodes two vertical
// pixels, so the output is maxHeight rows tall and maxWidth columns wide at most.
func Render(imagePath string, maxWidth, maxHeight int) (string, error) {
	if maxWidth < 1 {
		maxWidth = 1
	}
	if maxHeight < 1 {
		maxHeight = 1
	}

	f, err := os.Open(imagePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("decoding image: %w", err)
	}

	// Each terminal row represents 2 pixel rows (upper/lower half-block),
	// so the pixel grid height is 2 * maxHeight.
	pixelH := maxHeight * 2
	pixelW := maxWidth

	// Fit the image into the pixel grid while preserving aspect ratio.
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	scaleW := float64(pixelW) / float64(srcW)
	scaleH := float64(pixelH) / float64(srcH)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}

	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	// Resize with Lanczos for smooth, high-quality downscaling.
	resized := imaging.Resize(src, newW, newH, imaging.Lanczos)

	// Render pairs of pixel rows as half-block characters.
	// ▀ (U+2580) with fg = top pixel color and bg = bottom pixel color.
	var buf strings.Builder
	for y := 0; y < newH; y += 2 {
		if y > 0 {
			buf.WriteByte('\n')
		}
		for x := 0; x < newW; x++ {
			top := resized.At(x, y)
			tr, tg, tb, ta := top.RGBA()
			topOpaque := ta > 0x8000

			if y+1 < newH {
				bot := resized.At(x, y+1)
				br, bg, bb, ba := bot.RGBA()
				botOpaque := ba > 0x8000

				switch {
				case topOpaque && botOpaque:
					// Both pixels visible — ▀ with fg=top, bg=bottom
					fmt.Fprintf(&buf, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀\x1b[0m",
						tr>>8, tg>>8, tb>>8,
						br>>8, bg>>8, bb>>8,
					)
				case topOpaque && !botOpaque:
					// Only top visible — ▀ with fg=top, default bg (transparent)
					fmt.Fprintf(&buf, "\x1b[38;2;%d;%d;%dm▀\x1b[0m",
						tr>>8, tg>>8, tb>>8,
					)
				case !topOpaque && botOpaque:
					// Only bottom visible — ▄ with fg=bottom, default bg (transparent)
					fmt.Fprintf(&buf, "\x1b[38;2;%d;%d;%dm▄\x1b[0m",
						br>>8, bg>>8, bb>>8,
					)
				default:
					// Both transparent — just a space
					buf.WriteByte(' ')
				}
			} else {
				// Odd last row — only top pixel.
				if topOpaque {
					fmt.Fprintf(&buf, "\x1b[38;2;%d;%d;%dm▀\x1b[0m",
						tr>>8, tg>>8, tb>>8,
					)
				} else {
					buf.WriteByte(' ')
				}
			}
		}
	}

	return buf.String(), nil
}
