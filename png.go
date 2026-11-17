package fyne_iconkit

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

// PNG rasterizes the named icon to PNG. size is the length of the longer edge
// in pixels; the aspect ratio of the SVG view box is preserved.
//
// Icons of stainable sets are stained with the first stain color, or black if
// none is given. For other sets stain colors are ignored.
func (s *Set) PNG(name string, size int, stainColor ...color.Color) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("icon %q: invalid PNG size %d", name, size)
	}
	src, err := s.Source(name)
	if err != nil {
		return nil, err
	}
	if s.stainable {
		var c color.Color = color.Black
		if len(stainColor) > 0 && stainColor[0] != nil {
			c = stainColor[0]
		}
		src = Stain(src, c)
	}

	img, err := rasterize(src, size)
	if err != nil {
		return nil, fmt.Errorf("icon %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("icon %q: %w", name, err)
	}
	return buf.Bytes(), nil
}

func rasterize(src []byte, size int) (img *image.NRGBA, err error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(src), oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, err
	}
	vb := icon.ViewBox
	if vb.W <= 0 || vb.H <= 0 {
		return nil, errors.New("svg has no usable view box")
	}

	w, h := size, size
	if vb.W > vb.H {
		h = max(1, int(float64(size)*vb.H/vb.W+0.5))
	} else if vb.H > vb.W {
		w = max(1, int(float64(size)*vb.W/vb.H+0.5))
	}

	setTarget(icon, float64(w), float64(h))
	img = image.NewNRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())

	defer func() {
		if r := recover(); r != nil {
			img, err = nil, fmt.Errorf("svg rendering panicked: %v", r)
		}
	}()
	icon.Draw(rasterx.NewDasher(w, h, scanner), 1)
	return img, nil
}

// setTarget makes icon draw its view box into a w×h rectangle at the origin.
// Unlike icon.SetTarget it also scales stroke widths and dashes, which oksvg
// leaves in target pixels.
func setTarget(icon *oksvg.SvgIcon, w, h float64) {
	icon.SetTarget(0, 0, w, h)
	scale := math.Sqrt(w / icon.ViewBox.W * h / icon.ViewBox.H)
	for i := range icon.SVGPaths {
		p := &icon.SVGPaths[i]
		p.LineWidth *= scale
		p.DashOffset *= scale
		dash := make([]float64, len(p.Dash))
		for n, d := range p.Dash {
			dash[n] = d * scale
		}
		p.Dash = dash
	}
}
