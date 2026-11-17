package fyne_iconkit

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"
)

// outlineScale enlarges the view box while outlining, so curves are flattened
// finely enough for icons far larger than their nominal size.
const outlineScale = 16

// Outline converts a monochrome SVG into one that only consists of filled
// <path> elements with fill="currentColor": strokes become filled outlines.
// Translucent parts get a fill-opacity.
//
// Fyne recolors SVGs by replacing their fill colors. On the way it drops
// stroke colors, stroke attributes of the root element and elements like
// <line> or <polyline>, so stroked icons vanish or turn into blobs in
// theme.NewThemedResource, NewColoredResource, NewDisabledResource and in
// widgets that use them (disabled buttons and menu items, important buttons,
// selected tabs). Outlined icons are recolored correctly, like Fyne's own
// icons; only the opacity of translucent parts is lost.
//
// Fyne also does not scale stroke widths with the rendered size; outlines
// scale like any other shape.
//
// Curves are flattened to lines that deviate at most 1/100 of a view box
// unit from the exact shape.
func Outline(src []byte) (out []byte, err error) {
	// oksvg does not know currentColor; the shape does not depend on it.
	icon, err := oksvg.ReadIconStream(bytes.NewReader(Stain(src, color.Black)), oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, err
	}
	vb := icon.ViewBox
	if vb.W <= 0 || vb.H <= 0 {
		return nil, errors.New("svg has no usable view box")
	}
	w, h := vb.W*outlineScale, vb.H*outlineScale
	setTarget(icon, w, h)

	rec := &recorder{}
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("svg outlining panicked: %v", r)
		}
	}()
	icon.Draw(rasterx.NewDasher(int(math.Ceil(w)), int(math.Ceil(h)), rec), 1)

	width, height := rootSize(src)
	if width == "" {
		width = num(vb.W)
	}
	if height == "" {
		height = num(vb.H)
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="%s %s %s %s">`,
		width, height, num(vb.X), num(vb.Y), num(vb.W), num(vb.H))
	for _, p := range rec.paths {
		b.WriteString(`<path fill="currentColor"`)
		if p.opacity < 1 {
			fmt.Fprintf(&b, ` fill-opacity="%s"`, num(p.opacity))
		}
		if !p.nonZero {
			b.WriteString(` fill-rule="evenodd"`)
		}
		b.WriteString(` d="`)
		for n, loop := range p.loops {
			for i, pt := range loop {
				switch {
				case i == 0 && n > 0:
					b.WriteString("ZM")
				case i == 0:
					b.WriteString("M")
				case i == 1:
					b.WriteString("L")
				default:
					b.WriteString(" ")
				}
				b.WriteString(num(vb.X + float64(pt.X)/64/outlineScale))
				b.WriteString(" ")
				b.WriteString(num(vb.Y + float64(pt.Y)/64/outlineScale))
			}
		}
		b.WriteString(`Z"/>`)
	}
	b.WriteString("</svg>")
	return b.Bytes(), nil
}

// rootSize returns the width and height attributes of the root element.
func rootSize(src []byte) (width, height string) {
	var root struct {
		Width  string `xml:"width,attr"`
		Height string `xml:"height,attr"`
	}
	if xml.Unmarshal(src, &root) != nil {
		return "", ""
	}
	return root.Width, root.Height
}

func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

type outlinePath struct {
	loops   [][]fixed.Point26_6
	opacity float64
	nonZero bool
}

// recorder is a rasterx.Scanner that records the edges rasterx generates for
// fills and strokes instead of rasterizing them, and joins them to closed
// loops on Draw.
type recorder struct {
	runs    [][]fixed.Point26_6
	opacity float64
	nonZero bool
	paths   []outlinePath
}

var _ rasterx.Scanner = (*recorder)(nil)

func (r *recorder) Start(a fixed.Point26_6) {
	r.runs = append(r.runs, []fixed.Point26_6{a})
}

func (r *recorder) Line(b fixed.Point26_6) {
	if len(r.runs) == 0 {
		r.Start(b)
		return
	}
	run := &r.runs[len(r.runs)-1]
	if (*run)[len(*run)-1] != b {
		*run = append(*run, b)
	}
}

func (r *recorder) Draw() {
	if r.opacity > 0 {
		if loops := joinRuns(r.runs); len(loops) > 0 {
			r.paths = append(r.paths, outlinePath{loops: loops, opacity: r.opacity, nonZero: r.nonZero})
		}
	}
	r.runs = nil
}

func (r *recorder) GetPathExtent() fixed.Rectangle26_6 {
	ext := fixed.Rectangle26_6{Min: fixed.Point26_6{X: math.MaxInt32, Y: math.MaxInt32}, Max: fixed.Point26_6{X: math.MinInt32, Y: math.MinInt32}}
	for _, run := range r.runs {
		for _, p := range run {
			ext.Min.X, ext.Min.Y = min(ext.Min.X, p.X), min(ext.Min.Y, p.Y)
			ext.Max.X, ext.Max.Y = max(ext.Max.X, p.X), max(ext.Max.Y, p.Y)
		}
	}
	return ext
}

func (r *recorder) SetBounds(int, int) {}

func (r *recorder) SetColor(c interface{}) {
	r.opacity = 1 // gradients count as opaque
	if c, ok := c.(color.Color); ok {
		_, _, _, a := c.RGBA()
		r.opacity = float64(a) / 0xffff
	}
}

func (r *recorder) SetWinding(nonZero bool) { r.nonZero = nonZero }

func (r *recorder) Clear() { r.runs = nil }

func (r *recorder) SetClip(image.Rectangle) {}

// joinRuns chains the recorded runs of edges to closed loops. rasterx emits
// strokes as separate runs whose end points match exactly; a loop that cannot
// be closed is closed with a straight line.
func joinRuns(runs [][]fixed.Point26_6) [][]fixed.Point26_6 {
	starts := map[fixed.Point26_6][]int{}
	for i, run := range runs {
		if len(run) > 1 {
			starts[run[0]] = append(starts[run[0]], i)
		}
	}
	used := make([]bool, len(runs))
	take := func(p fixed.Point26_6) (int, bool) {
		for len(starts[p]) > 0 {
			i := starts[p][0]
			starts[p] = starts[p][1:]
			if !used[i] {
				used[i] = true
				return i, true
			}
		}
		return 0, false
	}
	var loops [][]fixed.Point26_6
	for i, run := range runs {
		if used[i] || len(run) < 2 {
			continue
		}
		used[i] = true
		loop := append([]fixed.Point26_6(nil), run...)
		for loop[len(loop)-1] != loop[0] {
			j, ok := take(loop[len(loop)-1])
			if !ok {
				break
			}
			loop = append(loop, runs[j][1:]...)
		}
		if loop[len(loop)-1] == loop[0] {
			loop = loop[:len(loop)-1]
		}
		if loop = simplify(loop); len(loop) > 2 {
			loops = append(loops, loop)
		}
	}
	return loops
}

// maxDeviation is the largest distance (in 1/64 pixels of the enlarged view
// box) simplify moves an outline: 1/100 of a view box unit.
const maxDeviation = 64 * outlineScale / 100

// simplify removes points of a closed loop that deviate less than
// maxDeviation from the simplified outline (Douglas-Peucker).
func simplify(loop []fixed.Point26_6) []fixed.Point26_6 {
	if len(loop) < 4 {
		return loop
	}
	// Split the loop at the point farthest from its first point.
	far, farDist := 0, int64(-1)
	for i, p := range loop {
		dx, dy := int64(p.X-loop[0].X), int64(p.Y-loop[0].Y)
		if d := dx*dx + dy*dy; d > farDist {
			far, farDist = i, d
		}
	}
	closed := append(append([]fixed.Point26_6(nil), loop...), loop[0])
	keep := make([]bool, len(closed))
	keep[0], keep[far], keep[len(closed)-1] = true, true, true
	type span struct{ from, to int }
	stack := []span{{0, far}, {far, len(closed) - 1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		worst, worstDist := -1, float64(maxDeviation)
		for i := s.from + 1; i < s.to; i++ {
			if d := distance(closed[i], closed[s.from], closed[s.to]); d > worstDist {
				worst, worstDist = i, d
			}
		}
		if worst >= 0 {
			keep[worst] = true
			stack = append(stack, span{s.from, worst}, span{worst, s.to})
		}
	}
	out := loop[:0:0]
	for i, p := range closed[:len(closed)-1] {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}

// distance returns the distance of p from the segment a-b.
func distance(p, a, b fixed.Point26_6) float64 {
	px, py := float64(p.X-a.X), float64(p.Y-a.Y)
	bx, by := float64(b.X-a.X), float64(b.Y-a.Y)
	l := bx*bx + by*by
	if l > 0 {
		t := math.Max(0, math.Min(1, (px*bx+py*by)/l))
		px, py = px-t*bx, py-t*by
	}
	return math.Hypot(px, py)
}
