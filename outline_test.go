package fyne_iconkit

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"testing/fstest"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// strokedSVG mixes the elements and stroke styles used by Tabler and Lucide.
const strokedSVG = `<svg
  xmlns="http://www.w3.org/2000/svg"
  width="24"
  height="24"
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
>
  <path stroke="none" d="M0 0h24v24H0z" fill="none" />
  <path d="M12.983 21.186a1 1 0 0 1 -1.966 0 10 10 0 0 0 -8.203 -8.203" />
  <line x1="4" y1="4" x2="20" y2="4" />
  <polyline points="4 8 12 12 20 8" />
  <circle cx="12" cy="17" r="2" fill="currentColor" />
  <rect x="2" y="2" width="20" height="20" rx="2" stroke-dasharray="4 2" />
</svg>`

func coverage(img *image.NRGBA) float64 {
	sum := 0.0
	for i := 3; i < len(img.Pix); i += 4 {
		sum += float64(img.Pix[i]) / 0xff
	}
	return sum
}

func TestOutline(t *testing.T) {
	out, err := Outline([]byte(strokedSVG))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"stroke", "<line", "<polyline", "<circle", "<rect"} {
		if bytes.Contains(out, []byte(unwanted)) {
			t.Errorf("outline contains %q: %s", unwanted, out)
		}
	}
	if !bytes.HasPrefix(out, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">`)) {
		t.Errorf("unexpected root element: %.120s", out)
	}
	// One per fill or stroke: the circle is filled and stroked.
	if n := bytes.Count(out, []byte(`<path fill="currentColor"`)); n != 6 {
		t.Errorf("%d filled paths, want 6", n)
	}

	// The outline covers the same pixels as the stroked original.
	for _, size := range []int{24, 96} {
		want, err := rasterize(Stain([]byte(strokedSVG), color.Black), size)
		if err != nil {
			t.Fatal(err)
		}
		got, err := rasterize(Stain(out, color.Black), size)
		if err != nil {
			t.Fatal(err)
		}
		if w, g := coverage(want), coverage(got); math.Abs(w-g) > 0.01*w {
			t.Errorf("size %d: outline covers %.1f px, original %.1f px", size, g, w)
		}
	}
}

func TestOutlineErrors(t *testing.T) {
	for _, src := range []string{"", "not svg", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`} {
		if _, err := Outline([]byte(src)); err == nil {
			t.Errorf("Outline(%q) succeeded", src)
		}
	}
}

func TestStrokesScale(t *testing.T) {
	line := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" stroke="#000000" stroke-width="2"><path d="M4 12H20"/></svg>`)
	small, err := rasterize(line, 24)
	if err != nil {
		t.Fatal(err)
	}
	large, err := rasterize(line, 96)
	if err != nil {
		t.Fatal(err)
	}
	// 16×2 px at 24 px, 64×8 px at 96 px.
	if s, l := coverage(small), coverage(large); math.Abs(l/s-16) > 0.5 {
		t.Errorf("coverage grows by %.1f from 24 to 96 px, want 16", l/s)
	}
}

// TestIconSurvivesThemedResource checks that Fyne can recolor stained icons:
// it replaces fill colors and drops strokes and <line>/<polyline> elements.
func TestIconSurvivesThemedResource(t *testing.T) {
	newTestApp(t)
	set := NewStainableSet(testMapFS("x", strokedSVG), "icons")
	res := set.MustIcon("x", color.Black)
	want, err := rasterize(res.Content(), 48)
	if err != nil {
		t.Fatal(err)
	}
	for name, wrapped := range map[string]fyne.Resource{
		"themed":   theme.NewThemedResource(res),
		"disabled": theme.NewDisabledResource(res),
		"error":    theme.NewErrorThemedResource(res),
	} {
		content := wrapped.Content()
		if strings.Contains(string(content), "#000000") {
			t.Errorf("%s: not recolored: %.200s", name, content)
		}
		got, err := rasterize(content, 48)
		if err != nil {
			t.Fatal(err)
		}
		if w, g := coverage(want), coverage(got); math.Abs(w-g) > 0.01*w {
			t.Errorf("%s: covers %.1f px, stained icon %.1f px", name, g, w)
		}
	}
}

func TestOutlineFallback(t *testing.T) {
	newTestApp(t)
	broken := `<svg xmlns="http://www.w3.org/2000/svg" stroke="currentColor"><path d="M0 0L1 1"/></svg>`
	res := NewStainableSet(testMapFS("x", broken), "icons").MustIcon("x", color.Black)
	if string(res.Content()) != strings.ReplaceAll(broken, "currentColor", "#000000") {
		t.Errorf("icon without view box is not served as is: %s", res.Content())
	}
}

func testMapFS(name, svg string) fstest.MapFS {
	return fstest.MapFS{"icons/" + name + ".svg": {Data: []byte(svg)}}
}
