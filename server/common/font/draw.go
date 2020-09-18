package font

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/coyove/nnn/server/common"
)

var BasePlane image.Image

func init() {
	path := filepath.Join(os.TempDir(), "pixii12.png")

	var buf []byte
	if _, err := os.Stat(path); err != nil {
		resp, err := http.Get("https://github.com/coyove/Pixii/raw/master/pixii-plane0.png")
		common.PanicIf(err, "%%err")
		defer resp.Body.Close()
		buf, _ = ioutil.ReadAll(resp.Body)
		common.PanicIf(ioutil.WriteFile(path, buf, 0777), "%%err")
	} else {
		buf, _ = ioutil.ReadFile(path)
	}

	img, err := png.Decode(bytes.NewReader(buf))
	common.PanicIf(err, "%%err")
	common.PanicIf(img.Bounds().Dx() != 3072, "font: incorrect base plane: %v", img.Bounds())

	BasePlane = img
}

type Textbox struct {
	LineSpace int
	CharSpace int
	Margin    int
	TabWidth  int
	Width     int
	Gray      bool
	Underline bool

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
			tb.x = tb.Margin
			tb.y += tb.dy
		}
		tb.ensureHeight()

		if r > 0xffff {
			r = 0xfffd
		}

		switch r {
		case '\n':
			tb.x = tb.Margin
			tb.y += tb.dy
			continue
		case '\t':
			tb.x += tb.TabWidth * tb.dx
			continue
		}

		var pidx uint8 = 1
		if tb.Gray {
			pidx = 2
		}

		y, x := int(r/256), int(r%256)
		for xx := x * 12; xx < x*12+12; xx++ {
			for yy := y * 12; yy < y*12+12; yy++ {
				dx, dy := xx-x*12, yy-y*12
				if r, g, b, _ := BasePlane.At(xx, yy).RGBA(); r+g+b == 0 {
					i := tb.canvas.PixOffset(tb.x+dx, tb.y+dy)
					tb.canvas.Pix[i] = pidx
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
	}
}

func (tb *Textbox) End(w io.Writer) error {
	return png.Encode(w, tb.canvas)
}
