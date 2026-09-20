// The mark a site is known by: the favicon in a tab, the icon on a home
// screen, and the picture a shared link previews.
//
// This is the only writer here that produces something a person looks at
// rather than something a crawler parses, and the reason it exists is that
// two findings on a real site had no file to name. A tab with no favicon
// shows the blank-page glyph; a link with no og:image previews as a grey
// rectangle. Neither is a judgement call about the site — both are just
// missing — and a writer is what missing files are for.
//
// It cannot be a photograph of your site, so it is the next honest thing: a
// mark derived from the site's own origin, the way a version control host
// draws one for an account with no avatar. Deterministic, so the same site
// gets the same mark on every machine and every rebuild, and a rebuild that
// changed nothing writes the same bytes; distinct, so two sites open side by
// side are told apart.
//
// A PNG rather than an SVG, and that is the whole decision. An SVG favicon is
// fine in every current browser — but no social scraper renders one, so an
// SVG mark would close the favicon finding and leave the link preview exactly
// as empty as it was. One file has to do both jobs and only PNG does.
package seo

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/seo/checkers"
)

const (
	// iconFile is spelled once: the registry produces it, the head writer
	// points two <link> tags and an og:image at it, and the _headers writer
	// gives it its own cache rule. Four spellings of one name is four chances
	// to write a tag pointing at a file nobody wrote.
	iconFile = "icon.png"
	// 512 square. Large enough that a scraper keeps it — a link preview drops
	// an image under 200×200 without saying so — and square, so a browser can
	// scale it to 16px without letterboxing. A real social card is 1200×630
	// with the site's words on it, which is what --image is for.
	iconSize = 512
	iconMin  = 200
	iconGrid = 5  // cells across, mirrored down the middle
	iconPad  = 56 // 5*80 + 2*56 == 512
	iconCell = (iconSize - 2*iconPad) / iconGrid
)

// iconPaper is what the mark is drawn on. Opaque rather than transparent,
// because a transparent PNG lands on whatever colour the scraper chose, and a
// dark mark on a dark card is no mark at all.
var iconPaper = color.NRGBA{R: 0xf5, G: 0xf5, B: 0xf5, A: 0xff}

// writeIcon draws the site's mark: a five-by-five grid, mirrored down the
// middle, lit from a hash of the site's own origin.
//
// Symmetry is the whole trick. The same fifteen random bits laid out
// asymmetrically read as noise; mirrored, they read as a mark, which is why
// every identicon ever drawn is symmetrical.
func writeIcon(s Site) (content, covered string, err error) {
	seed := sha256.Sum256([]byte(cli.Or(s.Origin, s.Title)))
	mark := hueRGB(float64(seed[0]) / 256 * 360)
	img := image.NewPaletted(image.Rect(0, 0, iconSize, iconSize), color.Palette{iconPaper, mark})
	// Two colours and a palette, not truecolour: the whole 512-square file is
	// then about a kilobyte, which is small enough that nobody has to think
	// about whether shipping it costs anything.
	bits := uint32(seed[1])<<16 | uint32(seed[2])<<8 | uint32(seed[3])
	cells := 0
	for col := range iconGrid/2 + 1 {
		for row := range iconGrid {
			if bits&(1<<(col*iconGrid+row)) == 0 {
				continue
			}
			cells += block(img, col, row)
			if mirror := iconGrid - 1 - col; mirror != col {
				cells += block(img, mirror, row)
			}
		}
	}
	// One origin in 32,768 hashes to fifteen zero bits, and a blank icon is a
	// failure nothing would ever report: the file is a valid PNG of the right
	// size and every checker asks only that it exists. The centre column is
	// the mark in that case, so there is always something in the tab.
	if cells == 0 {
		for row := range iconGrid {
			cells += block(img, iconGrid/2, row)
		}
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		return "", "", err
	}
	return buf.String(), fmt.Sprintf("%d×%d PNG, %s lit", iconSize, iconSize, cli.Plural(cells, "cell")), nil
}

// block paints one cell of the grid and returns the one it lit, so the caller
// counts by adding rather than by saying the number in a second place.
func block(img *image.Paletted, col, row int) int {
	x0, y0 := iconPad+col*iconCell, iconPad+row*iconCell
	for y := y0; y < y0+iconCell; y++ {
		for x := x0; x < x0+iconCell; x++ {
			img.SetColorIndex(x, y, 1)
		}
	}
	return 1
}

// hueRGB is one hue at a fixed saturation and lightness.
//
// Only the hue comes from the hash. The other two are pinned because a random
// RGB triple is as likely to come out pale yellow — invisible on the paper it
// is drawn on — as it is to come out something a reader can see. Fixing
// saturation and lightness makes every mark legible and leaves the hash the
// one thing it is good at: telling two sites apart.
func hueRGB(deg float64) color.NRGBA {
	const sat, light = 0.55, 0.42
	c := (1 - math.Abs(2*light-1)) * sat
	x := c * (1 - math.Abs(math.Mod(deg/60, 2)-1))
	m := light - c/2
	var r, g, b float64
	switch int(deg) / 60 {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	to := func(v float64) uint8 { return uint8(math.Round((v + m) * 255)) }
	return color.NRGBA{R: to(r), G: to(g), B: to(b), A: 0xff}
}

// validateIcon reads the bytes back the way a browser and a scraper would: is
// it a PNG at all, and is it big enough that a link preview keeps it.
//
// Presence is not enough here, unlike the other artifacts. Facebook and
// LinkedIn drop an og:image under 200×200 and say nothing about it, so a file
// that exists and is too small fails in exactly the place nobody looks.
func validateIcon(name, content, origin string) (found []cli.Finding, covered string) {
	cfg, err := png.DecodeConfig(strings.NewReader(content))
	if err != nil {
		return []cli.Finding{{
			Severity: cli.SevError,
			ID:       "icon-not-png",
			Message:  name + " is not a PNG: " + err.Error(),
			Fix:      "a browser and a social scraper both read this as an image — write it with: dev seo write <dir>",
		}}, "unreadable"
	}
	if cfg.Width < iconMin || cfg.Height < iconMin {
		found = append(found, cli.Finding{
			Severity: cli.SevWarning,
			ID:       "icon-too-small",
			Message:  fmt.Sprintf("%s is %d×%d", name, cfg.Width, cfg.Height),
			Fix: fmt.Sprintf("a link preview silently drops an image under %d×%d — %s",
				iconMin, iconMin, checkers.DocEssentials),
		})
	}
	return found, fmt.Sprintf("%d×%d PNG", cfg.Width, cfg.Height)
}
