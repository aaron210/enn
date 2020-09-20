package font

import (
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"

	"github.com/coyove/enn/server/common"
)

var BasePlane image.Image

func init() {
	img, err := png.Decode(base64.NewDecoder(base64.StdEncoding, strings.NewReader(p)))
	common.PanicIf(err, "%%err")
	common.PanicIf(img.Bounds().Dx() != 3072, "font: incorrect base plane: %v", img.Bounds())

	BasePlane = img
}

type Textbox struct {
	LineSpace              int
	CharSpace              int
	Margin                 int
	TabWidth               int
	Width                  int
	Indent                 int
	Gray, Red, Blue, Green bool
	Underline              bool
	Strikeline             bool
	Bold                   bool

	canvas      *image.Paletted
	x, y        int
	rightmost   int
	dx, dx2, dy int
}

func (tb *Textbox) Begin() {
	tb.x = tb.Margin
	tb.y = tb.Margin
	tb.dx = 6 + tb.CharSpace
	tb.dx2 = 12 + tb.CharSpace
	tb.dy = 12 + tb.LineSpace
	tb.canvas = image.NewPaletted(image.Rect(0, 0, tb.Width, tb.Margin*2), color.Palette{
		color.White,
		color.Black,
		color.Gray16{0x8000},
		color.RGBA{255, 0, 0, 255},
		color.RGBA{0, 0x96, 0x88, 255},
		color.RGBA{0, 0, 255, 255},
	})
	tb.rightmost = tb.canvas.Bounds().Dx() - tb.Margin - tb.dx

	if tb.TabWidth == 0 {
		tb.TabWidth = 4
	}
}

func (tb *Textbox) ensureHeight() {
	diff := tb.y + tb.dy + tb.Margin - tb.canvas.Bounds().Dy()
	if diff > 0 {
		for i := 0; i < diff*tb.canvas.Stride; i++ {
			tb.canvas.Pix = append(tb.canvas.Pix, 0)
		}
		tb.canvas.Rect.Max.Y += diff
	}
}

func (tb *Textbox) Write(text string) {
	for _, r := range text {
		if tb.x > tb.rightmost {
			tb.x = tb.Margin + tb.Indent*tb.dx
			tb.y += tb.dy
		}
		tb.ensureHeight()

		if r > 0xffff {
			r = 0xfffd
		}

		switch r {
		case '\n':
			tb.x = 1e10
			continue
		case '\t':
			tb.x += tb.TabWidth * tb.dx
			continue
		}

		var pidx uint8 = 1
		if tb.Gray {
			pidx = 2
		} else if tb.Red {
			pidx = 3
		} else if tb.Green {
			pidx = 4
		} else if tb.Blue {
			pidx = 5
		}

		y, x := int(r/256), int(r%256)
		safeset := func(pidx uint8, x, y int) {
			i := tb.canvas.PixOffset(x, y)
			if i < len(tb.canvas.Pix) {
				tb.canvas.Pix[i] = pidx
			}
		}
		for xx := x * 12; xx < x*12+12; xx++ {
			for yy := y * 12; yy < y*12+12; yy++ {
				dx, dy := xx-x*12, yy-y*12
				if r, g, b, _ := BasePlane.At(xx, yy).RGBA(); r+g+b == 0 {
					safeset(pidx, tb.x+dx, tb.y+dy)
					if tb.Bold {
						safeset(2, tb.x+dx+1, tb.y+dy)
						safeset(2, tb.x+dx, tb.y+dy+1)
						safeset(2, tb.x+dx+1, tb.y+dy+1)
					}
				}
			}
		}

		oldx := tb.x
		if r > 255 {
			tb.x += tb.dx2
		} else {
			tb.x += tb.dx
		}

		if tb.Underline {
			for xx := oldx; xx < tb.x; xx++ {
				tb.canvas.Pix[tb.canvas.PixOffset(xx, tb.y+12+1+1)] = pidx
			}
		}

		if tb.Strikeline {
			for xx := oldx; xx < tb.x; xx++ {
				tb.canvas.Pix[tb.canvas.PixOffset(xx, tb.y+12/2+xx%2)] = pidx
			}
		}
	}
}

func (tb *Textbox) End(w io.Writer) error {
	return png.Encode(w, tb.canvas)
}
