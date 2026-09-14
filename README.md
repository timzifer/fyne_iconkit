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

//go:generate go run github.com/timzifer/fyne_iconkit/cmd/iconconst -dir icons -prefix Icon -out icons_gen.go

//go:embed icons
var icons embed.FS

// NewStainableSet for monochrome icons using "currentColor",
// NewSet for multi-colored icons that are served as they are.
var set = fyne_iconkit.NewStainableSet(icons, "icons")

func MustIcon(name string, stain ...color.Color) fyne.Resource {
	return set.MustIcon(name, stain...)
}
```

A `Set` offers:

| Method | Description |
|---|---|
| `Icon(name, stain...)` | cached `fyne.Resource`; stainable sets return a `StainableResource` (default color: theme foreground) |
| `MustIcon(name, stain...)` | like `Icon`, logs via `fyne.LogError` and falls back to `theme.ErrorIcon()` |
| `Source(name)` / `StainedSource(name, c)` | raw or stained SVG bytes |
| `PNG(name, size, stain...)` | rasterized PNG, `size` = longer edge in pixels |
| `Names()` / `Has(name)` | list / check icons |

Unknown icons yield errors wrapping `fs.ErrNotExist`.

## Name constants

`cmd/iconconst` writes one constant per `*.svg` file:

| Flag | Default | |
|---|---|---|
| `-dir` | `icons` | directory with the SVG files |
| `-out` | `icons_gen.go` | output file |
| `-pkg` | `$GOPACKAGE` | package name |
| `-prefix` | `Icon` | identifier prefix |
| `-upper` | `false` | `gb-eng` → `FlagGB_ENG` instead of `FlagGbEng` |

## License

[MIT](LICENSE)
