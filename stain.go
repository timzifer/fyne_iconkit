package fyne_iconkit

import (
	"bytes"
	"fmt"
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// StainableResource is a monochrome icon resource that can be re-colored.
type StainableResource interface {
	fyne.Resource

	// StainColor returns the color the resource is currently stained with.
	StainColor() color.Color
	// Stain returns the same icon stained with c.
	Stain(c color.Color) (fyne.Resource, error)
	// MustStain is like Stain but returns theme.ErrorIcon() on failure.
	MustStain(c color.Color) fyne.Resource
}

type stainedResource struct {
	set     *Set
	name    string
	color   color.Color
	content []byte
}

var _ StainableResource = (*stainedResource)(nil)

// Name is unique per icon and color, so Fyne's render cache never mixes up
// differently stained variants of the same icon.
func (r *stainedResource) Name() string {
	return cacheKey(r.name, r.color) + svgExt
}

func (r *stainedResource) Content() []byte {
	return r.content
}

func (r *stainedResource) StainColor() color.Color {
	return r.color
}

func (r *stainedResource) Stain(c color.Color) (fyne.Resource, error) {
	return r.set.Icon(r.name, c)
}

func (r *stainedResource) MustStain(c color.Color) fyne.Resource {
	res, err := r.Stain(c)
	if err != nil {
		fyne.LogError("could not stain icon", err)
		return theme.ErrorIcon()
	}
	return res
}

var (
	currentColor       = []byte("currentColor")
	fillCurrentColor   = []byte(`fill="currentColor"`)
	strokeCurrentColor = []byte(`stroke="currentColor"`)
)

// Stain replaces every "currentColor" in src with c. Translucent colors add
// fill-opacity/stroke-opacity attributes, because SVG attributes do not
// reliably support colors with alpha.
func Stain(src []byte, c color.Color) []byte {
	nrgba := color.NRGBAModel.Convert(c).(color.NRGBA)
	hex := []byte(fmt.Sprintf("#%02x%02x%02x", nrgba.R, nrgba.G, nrgba.B))

	out := src
	if nrgba.A != 0xff {
		opacity := strconv.FormatFloat(float64(nrgba.A)/0xff, 'f', 3, 64)
		out = bytes.ReplaceAll(out, fillCurrentColor, []byte(fmt.Sprintf(`fill="%s" fill-opacity="%s"`, hex, opacity)))
		out = bytes.ReplaceAll(out, strokeCurrentColor, []byte(fmt.Sprintf(`stroke="%s" stroke-opacity="%s"`, hex, opacity)))
	}
	return bytes.ReplaceAll(out, currentColor, hex)
}

func hexRGBA(c color.Color) string {
	nrgba := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("%02x%02x%02x%02x", nrgba.R, nrgba.G, nrgba.B, nrgba.A)
}
