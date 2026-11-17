// Package iconupdate imports releases of an icon set published on npm into an
// icon package built on fyne_iconkit, keeping renamed icons usable.
//
// An icon package wraps it in a small command that describes its Source:
//
//	func main() {
//		iconupdate.Main(iconupdate.Source{
//			Name:    "Tabler Icons",
//			URL:     "https://tabler.io/icons",
//			Package: "@tabler/icons",
//			...
//		})
//	}
//
// and runs it with go generate before cmd/iconconst:
//
//	//go:generate go run ./internal/tablerupdate
//	//go:generate go run github.com/timzifer/fyne_iconkit/cmd/iconconst -dir icons -type icon -prefix Icon -out icons_gen.go
//
// Each run downloads a release (integrity-checked) and replaces the embedded
// SVG files and the upstream license. Renamed icons keep their old name as a
// deprecated alias until the next major release of the icon set, the way the
// upstream framework packages handle renames. Renames come from the source's
// own alias data, from names ("cart-x" → "shopping-cart-x"), from identical
// shapes and from manual hints in the state file.
//
// The state file (e.g. tabler.json) records the imported releases, manual
// rename hints and the aliases. A generated Go file declares the version
// constant, the alias variables and the alias map used by Lookup. Every
// imported release prepends a section to CHANGELOG.md listing renamed,
// removed, added and changed icons.
package iconupdate

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Source describes an icon set published as npm package and the files of
// the icon package that receives it.
type Source struct {
	// Name of the icon set in notes and comments, e.g. "Tabler Icons".
	Name string
	// URL of the icon set; the README must contain "[Name](URL) (vX.Y.Z)".
	URL string
	// Package is the npm package, e.g. "@tabler/icons".
	Package string
	// Dirs maps tarball directories (with trailing slash) to the suffix
	// appended to the names of their icons, e.g.
	// {"package/icons/filled/": "-filled"}.
	Dirs map[string]string
	// License is the path of the license in the tarball, e.g. "package/LICENSE".
	License string
	// Files are further tarball paths handed to Aliases.
	Files []string
	// CompareURL returns a link comparing two releases. Optional.
	CompareURL func(from, to string) string
	// Aliases returns the renames published by the icon set (old → new
	// name), given the requested Files and the names of all icons in the
	// tarball. Names may also be given as Go identifiers without prefix
	// ("AlertCircle"); they are matched by identifier. Icons the tarball
	// ships under an old name too are not imported as icons. Optional.
	Aliases func(ctx context.Context, version string, files map[string][]byte, icons []string) (map[string]string, error)
	// Clean normalizes an SVG before it is compared and written, e.g. by
	// removing a version comment. Optional.
	Clean func(svg []byte) []byte

	// Command runs the update tool, e.g. "go run ./internal/tablerupdate".
	Command string
	// GoPackage is the package of the generated file unless $GOPACKAGE is set.
	GoPackage string
	// VersionConst names the generated version constant, e.g. "TablerVersion".
	VersionConst string
	// StateFile, Out and LicenseFile are the default paths of the state
	// file, the generated Go file and the upstream license.
	StateFile, Out, LicenseFile string
}

const prefix = "Icon"

type options struct {
	version      string
	dir          string
	license      string
	readme       string
	stateFile    string
	out          string
	changelog    string
	notes        string
	pkg          string
	offline      bool
	release      string
	changelogURL string
}

// Main parses the command line flags and imports a release of src. It exits
// the program on errors.
func Main(src Source) {
	var o options
	tool := filepath.Base(src.Command)
	flag.StringVar(&o.version, "version", "latest", `release to import, e.g. "3.52.0", or "latest"`)
	flag.StringVar(&o.dir, "dir", "icons", "directory receiving the *.svg files")
	flag.StringVar(&o.license, "license", src.LicenseFile, `upstream license file ("" to skip)`)
	flag.StringVar(&o.readme, "readme", "README.md", fmt.Sprintf(`README whose "[%s](%s) (vX.Y.Z)" is updated`, src.Name, src.URL))
	flag.StringVar(&o.stateFile, "state", src.StateFile, "imported releases and deprecated aliases")
	flag.StringVar(&o.out, "out", src.Out, fmt.Sprintf("generated Go file with %s and the deprecated aliases", src.VersionConst))
	flag.StringVar(&o.changelog, "changelog", "CHANGELOG.md", `changelog receiving the release notes ("" to skip)`)
	flag.StringVar(&o.notes, "notes", "", "also write the release notes of an imported release to this file, shortened to fit a pull request body")
	flag.StringVar(&o.pkg, "pkg", "", fmt.Sprintf("package name of the generated file (default $GOPACKAGE or %s)", src.GoPackage))
	flag.BoolVar(&o.offline, "offline", false, "only regenerate the -out file from the state file and the icons directory")
	flag.StringVar(&o.changelogURL, "changelog-url", "CHANGELOG.md", `link to the changelog in shortened notes; "{version}" is replaced by the release`)
	flag.StringVar(&o.release, "release-notes", "", "only write the changelog section of the current release to this file, shortened to fit a GitHub release")
	flag.Parse()
	if o.pkg == "" {
		o.pkg = os.Getenv("GOPACKAGE")
	}
	if o.pkg == "" {
		o.pkg = src.GoPackage
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	u := &updater{src: src, o: o}
	if err := u.run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", tool, err)
		os.Exit(1)
	}
}

type updater struct {
	src Source
	o   options
}

func (u *updater) run(ctx context.Context) error {
	o := u.o
	st, err := u.loadState()
	if err != nil {
		return err
	}
	if o.release != "" {
		return u.writeReleaseNotes(st)
	}
	old, err := readIcons(o.dir)
	if err != nil {
		return err
	}
	if o.offline {
		return u.writeOut(st, keys(old))
	}

	rel, err := fetchRelease(ctx, u.src.Package, o.version)
	if err != nil {
		return err
	}
	data, err := download(ctx, rel.Dist.Tarball)
	if err != nil {
		return err
	}
	if err := verify(data, rel.Dist.Integrity); err != nil {
		return err
	}
	pkg, err := u.extract(data)
	if err != nil {
		return err
	}
	var upstream map[string]string
	if u.src.Aliases != nil {
		if upstream, err = u.src.Aliases(ctx, rel.Version, pkg.files, sortedKeys(pkg.icons)); err != nil {
			return err
		}
	}

	dropAliasFiles(pkg.icons, upstream)
	d := compare(old, pkg.icons, upstream, st.Renames)
	from := st.Version
	newRelease := rel.Version != st.Version
	if newRelease {
		if slices.Contains(st.Releases, rel.Version) {
			return fmt.Errorf("release %s was imported before %s already", rel.Version, st.Version)
		}
		st.Version = rel.Version
		st.Releases = append(st.Releases, rel.Version)
	}
	aliased, retargeted, dropped := u.apply(st, pkg.icons, d)

	// Validate the aliases before touching any file.
	src, err := u.generate(st, keys(pkg.icons))
	if err != nil {
		return err
	}
	if err := writeIcons(o.dir, pkg.icons); err != nil {
		return err
	}
	if err := os.WriteFile(o.out, src, 0o644); err != nil {
		return err
	}
	if err := st.save(o.stateFile); err != nil {
		return err
	}
	if o.license != "" {
		if err := os.WriteFile(o.license, pkg.license, 0o644); err != nil {
			return err
		}
	}
	if err := u.updateReadme(st.Version); err != nil {
		return err
	}

	fmt.Printf("%s: %s %s, %d icons (%d added, %d removed, %d renamed, %d changed, %d aliases dropped)\n",
		filepath.Base(u.src.Command), u.src.Name, st.Version, len(pkg.icons),
		len(d.Added), len(d.Removed), len(aliased), len(d.Changed), len(dropped))
	if !newRelease {
		return nil
	}

	n := notes{From: from, To: st.Version, Total: len(pkg.icons), Diff: d, Aliased: aliased,
		Retargeted: retargeted, Dropped: dropped}
	md := u.markdown(n)
	if o.changelog != "" {
		if err := prependChangelog(o.changelog, md); err != nil {
			return err
		}
	}
	if o.notes != "" {
		if err := os.WriteFile(o.notes, []byte(shorten(md, u.changelogURL(st))), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (u *updater) changelogURL(st *state) string {
	return strings.ReplaceAll(u.o.changelogURL, "{version}", st.Version)
}

func (u *updater) writeReleaseNotes(st *state) error {
	changelog, err := os.ReadFile(u.o.changelog)
	if err != nil {
		return err
	}
	section, err := u.changelogSection(lf(changelog), st.Version)
	if err != nil {
		return fmt.Errorf("%s: %w", u.o.changelog, err)
	}
	return os.WriteFile(u.o.release, []byte(shorten(section, u.changelogURL(st))), 0o644)
}

func (u *updater) writeOut(st *state, names map[string]bool) error {
	src, err := u.generate(st, names)
	if err != nil {
		return err
	}
	return os.WriteFile(u.o.out, src, 0o644)
}

func keys(icons map[string][]byte) map[string]bool {
	names := make(map[string]bool, len(icons))
	for name := range icons {
		names[name] = true
	}
	return names
}

// readIcons returns the *.svg files of dir by icon name.
func readIcons(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	icons := map[string][]byte{}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".svg")
		if !ok || e.IsDir() {
			continue
		}
		if icons[name], err = os.ReadFile(filepath.Join(dir, e.Name())); err != nil {
			return nil, err
		}
	}
	return icons, nil
}

// writeIcons makes dir contain exactly the given icons. Only *.svg files are
// written or removed; unchanged files are not touched.
func writeIcons(dir string, icons map[string][]byte) error {
	old, err := readIcons(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name := range old {
		if _, keep := icons[name]; !keep {
			if err := os.Remove(filepath.Join(dir, name+".svg")); err != nil {
				return err
			}
		}
	}
	for _, name := range sortedKeys(icons) {
		if old[name] != nil && string(old[name]) == string(icons[name]) {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name+".svg"), icons[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
