// Package fyne_iconkit provides the shared runtime behind the fyne_* icon
// packages: loading SVG icons from an fs.FS, caching them as fyne.Resource,
// staining monochrome icons with an arbitrary color and rasterizing them to PNG.
//
// Icon packages usually wrap a single package-level Set around an embedded
// directory and generate name constants with cmd/iconconst.
package fyne_iconkit

import (
	"fmt"
	"image/color"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

const svgExt = ".svg"

// Set is a collection of SVG icons stored in a directory of an fs.FS.
// All methods are safe for concurrent use.
type Set struct {
	fsys      fs.FS
	dir       string
	stainable bool

	mu    sync.Mutex
	cache map[string]fyne.Resource
}

// NewSet returns a Set of multi-colored icons (e.g. flags). Icons are served
// as they are; stain colors passed to Icon are ignored.
func NewSet(fsys fs.FS, dir string) *Set {
	return &Set{fsys: fsys, dir: dir, cache: map[string]fyne.Resource{}}
}

// NewStainableSet returns a Set of monochrome icons that use "currentColor".
// Icons are always stained before use; without an explicit color the theme's
// foreground color at the time of the call is used.
func NewStainableSet(fsys fs.FS, dir string) *Set {
	s := NewSet(fsys, dir)
	s.stainable = true
	return s
}

// Names returns the sorted names of all icons in the set.
func (s *Set) Names() []string {
	entries, err := fs.ReadDir(s.fsys, s.dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), svgExt) {
			names = append(names, strings.TrimSuffix(e.Name(), svgExt))
		}
	}
	slices.Sort(names)
	return names
}

// Has reports whether the set contains an icon with the given name.
func (s *Set) Has(name string) bool {
	_, err := s.Source(name)
	return err == nil
}

// Source returns the unmodified SVG content of the named icon. Names are the
// file names without ".svg". Unknown names yield an error wrapping
// fs.ErrNotExist.
func (s *Set) Source(name string) ([]byte, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return nil, fmt.Errorf("icon %q: %w", name, fs.ErrNotExist)
	}
	src, err := fs.ReadFile(s.fsys, path.Join(s.dir, name+svgExt))
	if err != nil {
		return nil, fmt.Errorf("icon %q: %w", name, err)
	}
	return src, nil
}

// StainedSource returns the SVG content of the named icon with every
// "currentColor" replaced by c.
func (s *Set) StainedSource(name string, c color.Color) ([]byte, error) {
	src, err := s.Source(name)
	if err != nil {
		return nil, err
	}
	return Stain(src, c), nil
}

// Icon returns the named icon as a cached fyne.Resource.
//
// For stainable sets the result is a StainableResource colored with the first
// stain color, or with the current theme foreground color if none is given.
// For other sets stain colors are ignored.
func (s *Set) Icon(name string, stainColor ...color.Color) (fyne.Resource, error) {
	if !s.stainable {
		return s.load(name, name, func(src []byte) fyne.Resource {
			return fyne.NewStaticResource(name+svgExt, src)
		})
	}

	var c color.Color
	if len(stainColor) > 0 && stainColor[0] != nil {
		c = stainColor[0]
	} else {
		c = foregroundColor()
	}
	return s.load(name, cacheKey(name, c), func(src []byte) fyne.Resource {
		return &stainedResource{set: s, name: name, color: c, content: Stain(src, c)}
	})
}

// MustIcon is like Icon but logs the error via fyne.LogError and returns
// theme.ErrorIcon() if the icon cannot be loaded.
func (s *Set) MustIcon(name string, stainColor ...color.Color) fyne.Resource {
	icon, err := s.Icon(name, stainColor...)
	if err != nil {
		fyne.LogError("could not load icon", err)
		return theme.ErrorIcon()
	}
	return icon
}

func (s *Set) load(name, key string, build func(src []byte) fyne.Resource) (fyne.Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if res, ok := s.cache[key]; ok {
		return res, nil
	}
	src, err := s.Source(name)
	if err != nil {
		return nil, err
	}
	res := build(src)
	s.cache[key] = res
	return res, nil
}

// foregroundColor avoids theme.Color and theme.ForegroundColor to stay
// compatible with Fyne 2.3 without using deprecated API.
func foregroundColor() color.Color {
	app := fyne.CurrentApp()
	if app == nil {
		return theme.DefaultTheme().Color(theme.ColorNameForeground, theme.VariantDark)
	}
	return app.Settings().Theme().Color(theme.ColorNameForeground, app.Settings().ThemeVariant())
}

func cacheKey(name string, c color.Color) string {
	return name + "_" + hexRGBA(c)
}
