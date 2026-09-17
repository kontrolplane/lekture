package image

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/muesli/termenv"
)

// profile resolves the terminal's color capability once. The pixel-art
// renderer writes color escapes directly rather than going through lipgloss,
// so it has to do its own degradation: emitting 24-bit color to a 16-color
// terminal produces garbage rather than a rough approximation.
var profile = sync.OnceValue(resolveProfile)

// resolveProfile determines the terminal's color capability from the
// environment. It deliberately does not rely on stdout being a terminal: under
// "lekture serve" the process stdout is not the viewer's terminal at all, so a
// tty check there would wrongly report no color.
func resolveProfile() termenv.Profile {
	if termenv.EnvNoColor() {
		return termenv.Ascii
	}
	if p := termenv.EnvColorProfile(); p != termenv.Ascii {
		return p
	}
	switch colorterm := strings.ToLower(os.Getenv("COLORTERM")); colorterm {
	case "truecolor", "24bit":
		return termenv.TrueColor
	}
	term := os.Getenv("TERM")
	switch {
	case term == "" || term == "dumb":
		return termenv.Ascii
	case strings.Contains(term, "truecolor"), strings.Contains(term, "direct"):
		return termenv.TrueColor
	case strings.Contains(term, "256color"):
		return termenv.ANSI256
	default:
		return termenv.ANSI
	}
}

// Supported reports whether the terminal can display pixel-art images at all.
// Without color the half-block glyphs carry no information, so callers should
// fall back to showing the image's alt text.
func Supported() bool { return profile() != termenv.Ascii }

// sequencer converts 8-bit RGB into an escape sequence for the current color
// profile, memoizing results since adjacent pixels repeat heavily.
type sequencer struct {
	profile termenv.Profile
	cache   map[uint32]string
}

func newSequencer() *sequencer {
	return &sequencer{profile: profile(), cache: make(map[uint32]string)}
}

func (s *sequencer) seq(r, g, b uint8, bg bool) string {
	key := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	if bg {
		key |= 1 << 24
	}
	if v, ok := s.cache[key]; ok {
		return v
	}
	var out string
	if c := s.profile.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b)); c != nil {
		// A profile without color returns an empty sequence; emitting
		// "\x1b[m" for it would reset styling on every cell.
		if seq := c.Sequence(bg); seq != "" {
			out = "\x1b[" + seq + "m"
		}
	}
	s.cache[key] = out
	return out
}

// decoded caches decoded source images. Decoding dominates the cost of
// rendering a slide (a large PNG can take ~70ms), and the same file is
// re-rendered on every terminal resize, so decode once and keep it.
var decoded sync.Map // decodeKey -> image.Image

// rendered caches finished ANSI output, so resizing back to a size already
// seen costs a map lookup instead of a resize plus a full cell walk.
var rendered sync.Map // renderKey -> string

type decodeKey struct {
	path    string
	size    int64
	modTime int64
}

type renderKey struct {
	decodeKey
	maxWidth  int
	maxHeight int
}

// stat returns the cache identity of path.
func stat(path string) (decodeKey, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return decodeKey{}, err
	}
	if fi.IsDir() {
		return decodeKey{}, fmt.Errorf("%s is a directory", path)
	}
	return decodeKey{path: path, size: fi.Size(), modTime: fi.ModTime().UnixNano()}, nil
}

// decode returns the decoded image for path, using a cached copy when the
// file”'s size and modification time are unchanged.
func decode(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var key decodeKey
	if fi, err := f.Stat(); err == nil {
		if fi.IsDir() {
			return nil, fmt.Errorf("%s is a directory", path)
		}
		key = decodeKey{path: path, size: fi.Size(), modTime: fi.ModTime().UnixNano()}
		if cached, ok := decoded.Load(key); ok {
			return cached.(image.Image), nil
		}
	}

	src, _, err := image.Decode(f) //nolint:staticcheck // format name unused
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}
	if key.path != "" {
		decoded.Store(key, src)
	}
	return src, nil
}

const (
	upperHalf = "▀"
	lowerHalf = "▄"
	reset     = "\x1b[0m"
	defaultBG = "\x1b[49m"
)

// alphaThreshold is the coverage above which a pixel counts as visible.
const alphaThreshold = 0x8000

// cell describes one rendered terminal cell.
type cell struct {
	set        bool
	hasBG      bool
	glyph      string
	fr, fg, fb uint8
	br, bg, bb uint8
}

// sample returns the 8-bit color of a pixel and whether it is opaque enough to
// draw. Colors are un-premultiplied, since the alpha-premultiplied values
// returned by RGBA would render semi-transparent pixels too dark.
func sample(img image.Image, x, y int) (r, g, b uint8, opaque bool) {
	cr, cg, cb, ca := img.At(x, y).RGBA()
	if ca == 0 {
		return 0, 0, 0, false
	}
	if ca < 0xffff {
		cr = cr * 0xffff / ca
		cg = cg * 0xffff / ca
		cb = cb * 0xffff / ca
	}
	return uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), ca > alphaThreshold
}

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

	// Serve a previously rendered result when the file and target size match.
	var rkey renderKey
	if dk, err := stat(imagePath); err == nil {
		rkey = renderKey{decodeKey: dk, maxWidth: maxWidth, maxHeight: maxHeight}
		if cached, ok := rendered.Load(rkey); ok {
			return cached.(string), nil
		}
	}

	src, err := decode(imagePath)
	if err != nil {
		return "", err
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
	// Resized images are not guaranteed to start at (0,0).
	min := resized.Bounds().Min

	seq := newSequencer()

	var buf strings.Builder
	var cur cell
	for y := 0; y < newH; y += 2 {
		if y > 0 {
			// Reset at end of line so the last color does not bleed.
			if cur.set {
				buf.WriteString(reset)
				cur = cell{}
			}
			buf.WriteByte('\n')
		}
		for x := 0; x < newW; x++ {
			tr, tg, tb, topOpaque := sample(resized, min.X+x, min.Y+y)

			var next cell
			switch {
			case y+1 >= newH:
				// Odd last row — only the top pixel exists.
				if topOpaque {
					next = cell{set: true, glyph: upperHalf, fr: tr, fg: tg, fb: tb}
				}
			default:
				br, bg, bb, botOpaque := sample(resized, min.X+x, min.Y+y+1)
				switch {
				case topOpaque && botOpaque:
					next = cell{set: true, glyph: upperHalf, fr: tr, fg: tg, fb: tb, hasBG: true, br: br, bg: bg, bb: bb}
				case topOpaque:
					next = cell{set: true, glyph: upperHalf, fr: tr, fg: tg, fb: tb}
				case botOpaque:
					next = cell{set: true, glyph: lowerHalf, fr: br, fg: bg, fb: bb}
				}
			}

			if !next.set {
				// Fully transparent — a plain space, with colors reset first.
				if cur.set {
					buf.WriteString(reset)
					cur = cell{}
				}
				buf.WriteByte(' ')
				continue
			}

			// Only emit escape sequences when the color actually changes from
			// the previous cell. Flat regions are the common case, and this
			// cuts the output size (and terminal write cost) substantially.
			if !cur.set || cur.fr != next.fr || cur.fg != next.fg || cur.fb != next.fb {
				buf.WriteString(seq.seq(next.fr, next.fg, next.fb, false))
			}
			switch {
			case next.hasBG && (!cur.hasBG || cur.br != next.br || cur.bg != next.bg || cur.bb != next.bb):
				buf.WriteString(seq.seq(next.br, next.bg, next.bb, true))
			case !next.hasBG && cur.hasBG:
				buf.WriteString(defaultBG)
			}
			buf.WriteString(next.glyph)
			cur = next
		}
	}
	if cur.set {
		buf.WriteString(reset)
	}

	out := buf.String()
	if rkey.path != "" {
		rendered.Store(rkey, out)
	}
	return out, nil
}
