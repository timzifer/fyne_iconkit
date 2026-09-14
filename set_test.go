package fyne_iconkit

import (
	"bytes"
	"errors"
	"image/color"
	"image/png"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

const (
	monoSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" fill="currentColor"/></svg>`
	flagSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 480"><path fill="#ff0000" d="M0 0h640v480H0z"/></svg>`
)

var testFS = fstest.MapFS{
	"icons/square.svg":  {Data: []byte(monoSVG)},
	"icons/b-icon.svg":  {Data: []byte(monoSVG)},
	"icons/readme.txt":  {Data: []byte("not an icon")},
	"icons/sub/x.svg":   {Data: []byte(monoSVG)},
	"flags/xx.svg":      {Data: []byte(flagSVG)},
	"secret/secret.svg": {Data: []byte(monoSVG)},
}

func newTestApp(t *testing.T) {
	t.Helper()
	test.NewApp()
	t.Cleanup(func() { test.NewApp() })
}

func TestNames(t *testing.T) {
	got := NewStainableSet(testFS, "icons").Names()
	if strings.Join(got, ",") != "b-icon,square" {
		t.Fatalf("Names() = %v", got)
	}
	if got := NewSet(testFS, "missing").Names(); len(got) != 0 {
		t.Fatalf("Names() of missing dir = %v", got)
	}
}

func TestSource(t *testing.T) {
	s := NewStainableSet(testFS, "icons")

	for _, name := range []string{"square"} {
		src, err := s.Source(name)
		if err != nil || string(src) != monoSVG {
			t.Errorf("Source(%q) = %q, %v", name, src, err)
		}
	}
	for _, name := range []string{"", "missing", "square.svg", "../secret/secret", `..\secret\secret`, "sub/x", ".", ".."} {
		if _, err := s.Source(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Source(%q) error = %v, want fs.ErrNotExist", name, err)
		}
	}
	if !s.Has("square") || s.Has("missing") {
		t.Error("Has() reports wrong result")
	}
}

func TestStain(t *testing.T) {
	src := []byte(monoSVG)

	opaque := string(Stain(src, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}))
	if strings.Contains(opaque, "currentColor") || strings.Count(opaque, "#123456") != 2 || strings.Contains(opaque, "opacity") {
		t.Errorf("opaque stain = %s", opaque)
	}

	translucent := string(Stain(src, color.NRGBA{R: 0xff, A: 0x80}))
	for _, want := range []string{`stroke="#ff0000" stroke-opacity="0.502"`, `fill="#ff0000" fill-opacity="0.502"`} {
		if !strings.Contains(translucent, want) {
			t.Errorf("translucent stain misses %s: %s", want, translucent)
		}
	}
}

func TestIconStatic(t *testing.T) {
	s := NewSet(testFS, "flags")

	res, err := s.Icon("xx", color.White)
	if err != nil {
		t.Fatal(err)
	}
	if res.Name() != "xx.svg" || string(res.Content()) != flagSVG {
		t.Errorf("Icon() = %s %s", res.Name(), res.Content())
	}
	if _, ok := res.(StainableResource); ok {
		t.Error("static icon must not be stainable")
	}
	if again, _ := s.Icon("xx"); again != res {
		t.Error("Icon() is not cached")
	}
}

func TestIconStainable(t *testing.T) {
	newTestApp(t)
	s := NewStainableSet(testFS, "icons")
	red := color.NRGBA{R: 0xff, A: 0xff}

	res, err := s.Icon("square", red)
	if err != nil {
		t.Fatal(err)
	}
	stainable, ok := res.(StainableResource)
	if !ok {
		t.Fatalf("Icon() returned %T, want StainableResource", res)
	}
	if res.Name() != "square_ff0000ff.svg" || !bytes.Contains(res.Content(), []byte("#ff0000")) {
		t.Errorf("Icon() = %s %s", res.Name(), res.Content())
	}
	if stainable.StainColor() != red {
		t.Errorf("StainColor() = %v", stainable.StainColor())
	}
	if again, _ := s.Icon("square", red); again != res {
		t.Error("Icon() is not cached per color")
	}

	halfRed := color.NRGBA{R: 0xff, A: 0x80}
	if other := stainable.MustStain(halfRed); other == res || other.Name() != "square_ff000080.svg" {
		t.Errorf("MustStain() with alpha = %s, want separate resource", other.Name())
	}

	settings := fyne.CurrentApp().Settings()
	want := color.NRGBAModel.Convert(settings.Theme().Color(theme.ColorNameForeground, settings.ThemeVariant()))
	def, err := s.Icon("square")
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(def.(StainableResource).StainColor()); got != want {
		t.Errorf("default stain = %v, want theme foreground %v", got, want)
	}
}

func TestMustIconFallback(t *testing.T) {
	newTestApp(t)
	if got := NewStainableSet(testFS, "icons").MustIcon("missing"); got != theme.ErrorIcon() {
		t.Errorf("MustIcon() = %v, want theme.ErrorIcon()", got)
	}
	if got := NewSet(testFS, "flags").MustIcon("missing"); got != theme.ErrorIcon() {
		t.Errorf("MustIcon() = %v, want theme.ErrorIcon()", got)
	}
}

func TestIconConcurrent(t *testing.T) {
	s := NewStainableSet(testFS, "icons")
	results := make([]fyne.Resource, 32)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = s.Icon("square", color.Black)
		}(i)
	}
	wg.Wait()
	for _, r := range results {
		if r == nil || r != results[0] {
			t.Fatal("concurrent Icon() calls returned different resources")
		}
	}
}

func TestPNG(t *testing.T) {
	t.Run("stainable", func(t *testing.T) {
		data, err := NewStainableSet(testFS, "icons").PNG("square", 48, color.NRGBA{B: 0xff, A: 0xff})
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != 48 || b.Dy() != 48 {
			t.Errorf("size = %v", b)
		}
		if got := color.NRGBAModel.Convert(img.At(24, 24)).(color.NRGBA); got != (color.NRGBA{B: 0xff, A: 0xff}) {
			t.Errorf("center pixel = %v, want blue", got)
		}
	})

	t.Run("static keeps aspect ratio", func(t *testing.T) {
		data, err := NewSet(testFS, "flags").PNG("xx", 64)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 48 {
			t.Errorf("size = %v, want 64x48", b)
		}
		if got := color.NRGBAModel.Convert(img.At(32, 24)).(color.NRGBA); got != (color.NRGBA{R: 0xff, A: 0xff}) {
			t.Errorf("center pixel = %v, want red", got)
		}
	})

	t.Run("errors", func(t *testing.T) {
		s := NewStainableSet(testFS, "icons")
		if _, err := s.PNG("square", 0); err == nil {
			t.Error("size 0 must fail")
		}
		if _, err := s.PNG("missing", 16); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("missing icon error = %v", err)
		}
	})
}
