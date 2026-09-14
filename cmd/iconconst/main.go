// Command iconconst generates Go constants for all *.svg icons in a directory.
//
// Typical use in an icon package:
//
//	//go:generate go run github.com/timzifer/fyne_iconkit/cmd/iconconst -dir icons -prefix Icon -out icons_gen.go
//
// The package name defaults to $GOPACKAGE, which go generate sets.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/timzifer/fyne_iconkit/internal/constgen"
)

func main() {
	var (
		dir    = flag.String("dir", "icons", "directory containing the *.svg files")
		out    = flag.String("out", "icons_gen.go", "output file")
		pkg    = flag.String("pkg", os.Getenv("GOPACKAGE"), "package name (default $GOPACKAGE)")
		prefix = flag.String("prefix", "Icon", "identifier prefix")
		upper  = flag.Bool("upper", false, `upper-case identifiers joined by "_" (e.g. FlagGB_ENG)`)
	)
	flag.Parse()

	cfg := constgen.Config{
		Package: *pkg,
		Dir:     filepath.ToSlash(filepath.Clean(*dir)),
		Prefix:  *prefix,
		Command: "iconconst " + strings.Join(os.Args[1:], " "),
	}
	if *upper {
		cfg.Case = constgen.Upper
	}

	src, err := constgen.Generate(os.DirFS("."), cfg)
	if err == nil {
		err = os.WriteFile(*out, src, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "iconconst:", err)
		os.Exit(1)
	}
}
