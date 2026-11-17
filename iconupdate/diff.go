package iconupdate

import (
	"bytes"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/timzifer/fyne_iconkit"
	"github.com/timzifer/fyne_iconkit/internal/constgen"
)

// Rename sources, recorded per alias.
const (
	sourceUpstream = "upstream" // alias data of the icon set
	sourceName     = "name"     // new name ends with "-<old name>", e.g. cart-x → shopping-cart-x
	sourceShape    = "shape"    // new icon draws exactly the same shape
	sourceManual   = "manual"   // renames or aliases added to the state file by hand
)

type rename struct {
	Target       string
	Source       string
	ShapeChanged bool // the new icon looks different than the old one
}

// iconDiff describes how one icon set turns into another.
type iconDiff struct {
	Added   []string // includes rename targets
	Removed []string // includes renamed icons
	Changed []string // icons on both sides whose shape differs
	Renames map[string]rename
}

// ident returns the identifier of an icon name, which also normalizes names
// given as identifiers ("alert-circle", "AlertCircle" → "IconAlertCircle").
// The prefix keeps names starting with a digit valid ("123" → "Icon123").
func ident(name string) string {
	id, err := constgen.Identifier(prefix, name, constgen.Camel)
	if err != nil {
		return ""
	}
	return id
}

// compare diffs two icon sets and finds the new names of removed icons:
// first from hints (manual renames), then from the upstream aliases, the name
// ("cart-x" → "shopping-cart-x", preferring added icons) and identical shapes.
// Only shapes count as changes: reformatted SVGs that draw the same are equal.
func compare(old, cur map[string][]byte, upstream, hints map[string]string) iconDiff {
	d := iconDiff{Renames: map[string]rename{}}
	for name, src := range cur {
		oldSrc, ok := old[name]
		switch {
		case !ok:
			d.Added = append(d.Added, name)
		case !sameShape(oldSrc, src):
			d.Changed = append(d.Changed, name)
		}
	}
	for name := range old {
		if _, ok := cur[name]; !ok {
			d.Removed = append(d.Removed, name)
		}
	}
	slices.Sort(d.Added)
	slices.Sort(d.Removed)
	slices.Sort(d.Changed)

	all := sortedKeys(cur)
	byIdent := map[string]string{}
	for _, name := range all {
		byIdent[ident(name)] = name
	}
	resolve := func(target string) (string, bool) {
		if cur[target] != nil {
			return target, true
		}
		name, ok := byIdent[ident(target)]
		return name, ok
	}
	upstreamByIdent := map[string]string{}
	for old, name := range upstream {
		upstreamByIdent[ident(old)] = name
	}
	byShape := map[string][]string{}
	for _, name := range d.Added {
		if s, err := shape(cur[name]); err == nil {
			byShape[s] = append(byShape[s], name)
		}
	}

	for _, name := range d.Removed {
		if target, ok := resolve(hints[name]); ok && hints[name] != "" {
			d.Renames[name] = rename{Target: target, Source: sourceManual}
			continue
		}
		if up, ok := upstreamByIdent[ident(name)]; ok {
			if target, ok := resolve(up); ok {
				d.Renames[name] = rename{Target: target, Source: sourceUpstream}
				continue
			}
		}
		suffix := func(n string) bool { return strings.HasSuffix(n, "-"+name) }
		if target, ok := unique(d.Added, suffix); ok {
			d.Renames[name] = rename{Target: target, Source: sourceName}
			continue
		}
		if target, ok := unique(all, suffix); ok {
			d.Renames[name] = rename{Target: target, Source: sourceName}
			continue
		}
		if s, err := shape(old[name]); err == nil && len(byShape[s]) == 1 {
			d.Renames[name] = rename{Target: byShape[s][0], Source: sourceShape}
		}
	}
	for name, r := range d.Renames {
		r.ShapeChanged = !sameShape(old[name], cur[r.Target])
		d.Renames[name] = r
	}
	return d
}

// dropAliasFiles removes icons that upstream ships only as copies under a
// deprecated name (e.g. lucide-static's home.svg next to house.svg), so they
// become deprecated aliases instead of icons. It returns their number.
func dropAliasFiles(icons map[string][]byte, upstream map[string]string) int {
	byIdent := map[string]string{}
	for name := range icons {
		byIdent[ident(name)] = name
	}
	dropped := 0
	// Exact names first: an alias may share its identifier with its target
	// ("arrow-down-01" → "arrow-down-0-1").
	find := func(name string) (string, bool) {
		if icons[name] != nil {
			return name, true
		}
		n, ok := byIdent[ident(name)]
		return n, ok
	}
	for _, old := range sortedKeys(upstream) {
		oldName, ok := find(old)
		if !ok {
			continue
		}
		targetName, ok := find(upstream[old])
		if ok && targetName != oldName && icons[oldName] != nil && icons[targetName] != nil {
			delete(icons, oldName)
			dropped++
		}
	}
	return dropped
}

func unique(names []string, match func(string) bool) (string, bool) {
	var found []string
	for _, n := range names {
		if match(n) {
			found = append(found, n)
		}
	}
	if len(found) != 1 {
		return "", false
	}
	return found[0], true
}

// sameShape reports whether two icons look the same: they draw the same
// elements, or their renderings differ in hardly any pixel (the same shape
// written differently, e.g. as <polyline> and as <path>).
func sameShape(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	sa, errA := shape(a)
	sb, errB := shape(b)
	if errA == nil && errB == nil && sa == sb {
		return true
	}
	return looksSame(a, b)
}

const (
	renderSize = 48
	// An icon looks different if more than this share of pixels differs
	// clearly in coverage. Calibrated on Lucide 0.93 → 1.74 renames: up to
	// about 4% are stroke and corner tweaks, from about 6% on the icon shows
	// visibly different details.
	maxDifferentPixels = 0.05
	clearDifference    = 0x60
)

// looksSame renders both icons and compares their coverage.
func looksSame(a, b []byte) bool {
	ia, errA := render(a)
	ib, errB := render(b)
	if errA != nil || errB != nil || ia.Bounds() != ib.Bounds() {
		return false
	}
	different := 0
	for i := 3; i < len(ia.Pix); i += 4 {
		if d := int(ia.Pix[i]) - int(ib.Pix[i]); d > clearDifference || d < -clearDifference {
			different++
		}
	}
	return float64(different) <= maxDifferentPixels*float64(len(ia.Pix)/4)
}

// render rasterizes an icon like fyne_iconkit does for PNG.
func render(svg []byte) (*image.NRGBA, error) {
	set := fyne_iconkit.NewStainableSet(fstest.MapFS{"i/i.svg": {Data: svg}}, "i")
	data, err := set.PNG("i", renderSize, color.Black)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		return nil, errors.New("unexpected image type")
	}
	return nrgba, nil
}

// geometry lists the attributes that define what an element draws.
var geometry = []string{"d", "points", "cx", "cy", "r", "rx", "ry", "x", "y", "width", "height", "x1", "y1", "x2", "y2"}

// shape returns a canonical description of the elements an SVG draws,
// ignoring the root element, styling, formatting and the invisible 24×24
// bounding-box path some icon sets add.
func shape(svg []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(svg))
	var out strings.Builder
	depth := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				continue
			}
			attrs := map[string]string{}
			for _, a := range t.Attr {
				attrs[a.Name.Local] = strings.Join(strings.Fields(a.Value), " ")
			}
			if t.Name.Local == "path" && attrs["d"] == "M0 0h24v24H0z" {
				continue
			}
			out.WriteString(t.Name.Local)
			for _, g := range geometry {
				if v, ok := attrs[g]; ok {
					out.WriteString(" " + g + "=" + v)
				}
			}
			out.WriteByte('\n')
		case xml.EndElement:
			depth--
		}
	}
	return out.String(), nil
}
