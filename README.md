# fyne_iconkit

[![CI](https://github.com/timzifer/fyne_iconkit/actions/workflows/ci.yml/badge.svg)](https://github.com/timzifer/fyne_iconkit/actions/workflows/ci.yml)

Shared runtime and code generator behind the [Fyne](https://fyne.io) icon packages

- [fyne_tabler](https://github.com/timzifer/fyne_tabler) – Tabler Icons
- [fyne_lucide](https://github.com/timzifer/fyne_lucide) – Lucide
- [fyne_flagicons](https://github.com/timzifer/fyne_flagicons) – country flags

You normally use one of those packages. Use `fyne_iconkit` directly to build
your own icon package. Requires Go 1.21+ and Fyne 2.3+.

## Icon sets

```go
package myicons

import (
	"embed"
	"image/color"

	"fyne.io/fyne/v2"
	"github.com/timzifer/fyne_iconkit"
)

//go:generate go run github.com/timzifer/fyne_iconkit/cmd/iconconst -dir icons -type icon -prefix Icon -out icons_gen.go

// icon is unexported and only created by the generated variables (IconHome, ...)
// or Lookup, so callers cannot pass names that do not exist.
type icon struct{ name string }

//go:embed icons
var icons embed.FS

// NewStainableSet for monochrome icons using "currentColor",
// NewSet for multi-colored icons that are served as they are.
var set = fyne_iconkit.NewStainableSet(icons, "icons")

func Icon(i icon, stain ...color.Color) fyne.Resource {
	return set.MustIcon(i.name, stain...)
}

func Lookup(name string) (icon, bool) {
	if !set.Has(name) {
		return icon{}, false
	}
	return icon{name}, true
}
```

A `Set` is addressed by plain names; wrapping packages hide that behind their
icon type. It offers:

| Method | Description |
|---|---|
| `Icon(name, stain...)` | cached `fyne.Resource`; stainable sets return a `StainableResource` (default color: theme foreground) |
| `MustIcon(name, stain...)` | like `Icon`, logs via `fyne.LogError` and falls back to `theme.ErrorIcon()` |
| `Source(name)` / `StainedSource(name, c)` | raw or stained SVG bytes |
| `PNG(name, size, stain...)` | rasterized PNG, `size` = longer edge in pixels |
| `Names()` / `Has(name)` | list / check icons |

Unknown icons yield errors wrapping `fs.ErrNotExist`.

### Stroked icons and Fyne's recoloring

Fyne recolors SVG icons by replacing their fill colors, e.g. in disabled
buttons and menu items, important buttons, selected tabs and
`theme.NewThemedResource`/`NewColoredResource`/`NewDisabledResource`. Stroked
icons like Tabler or Lucide do not survive that: strokes lose their attributes
and `<line>`/`<polyline>` elements are dropped, so icons turn into blobs or
vanish. Fyne also draws strokes with a fixed width regardless of the icon size.

`Icon` therefore serves stainable icons as `Outline`: strokes are converted
into filled shapes once per icon (about 1 ms, then cached). Outlined icons are
recolored correctly and scale like Fyne's own icons; only the opacity of
translucent parts is lost when Fyne recolors them. `Source` and
`StainedSource` still return the original SVG, and `PNG` scales strokes with
the requested size.

## Generated icon variables

`cmd/iconconst` writes one variable per `*.svg` file, e.g.
`IconArrowUp = icon{"arrow-up"}`. Struct values are used instead of string
constants because an untyped string literal would still be accepted wherever a
string-based type is expected.

| Flag | Default | |
|---|---|---|
| `-dir` | `icons` | directory with the SVG files |
| `-out` | `icons_gen.go` | output file |
| `-pkg` | `$GOPACKAGE` | package name |
| `-type` | `icon` | struct type with a single string field, declared in the package |
| `-prefix` | `Icon` | identifier prefix |
| `-upper` | `false` | `gb-eng` → `FlagGB_ENG` instead of `FlagGbEng` |

## Importing icon set releases

Package `iconupdate` imports releases of an icon set published on npm. An
icon package describes its source in a small command and runs it with
`go generate` before `iconconst`:

```go
// internal/tablerupdate/main.go
func main() {
	iconupdate.Main(iconupdate.Source{
		Name:    "Tabler Icons",
		URL:     "https://tabler.io/icons",
		Package: "@tabler/icons",
		Dirs:    map[string]string{"package/icons/outline/": "", "package/icons/filled/": "-filled"},
		License: "package/LICENSE",
		Aliases: fetchAliases, // renames published by the icon set
		// ... file names, version constant, Go package
	})
}
```

Each run downloads a release (integrity-checked), replaces the SVGs and the
license, and keeps renamed icons as `// Deprecated:` aliases until the next
major release of the icon set. Renames come from the icon set's own alias
data, from names (`cart-x` → `shopping-cart-x`), from identical looks and from
manual hints in the state file. Path data that oksvg cannot parse (compact arc
flags) is normalized. Every release adds a section to `CHANGELOG.md` listing
renamed, removed, added and visibly changed icons; `-release-notes` extracts
it for GitHub releases. See
[fyne_tabler](https://github.com/timzifer/fyne_tabler) and
[fyne_lucide](https://github.com/timzifer/fyne_lucide).

## License

[MIT](LICENSE)
