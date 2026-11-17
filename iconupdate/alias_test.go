package iconupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func icon(paths ...string) []byte {
	s := `<svg xmlns="http://www.w3.org/2000/svg" stroke="currentColor"><path stroke="none" d="M0 0h24v24H0z" fill="none"/>`
	for _, d := range paths {
		s += `<path d="` + d + `"/>`
	}
	return []byte(s + "</svg>")
}

func TestShape(t *testing.T) {
	v2 := []byte(`<svg class="icon icon-tabler-x" stroke="currentColor" fill="none">
  <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
  <path d="M5 12l-2  0" stroke-width="0" fill="currentColor" />
</svg>`)
	v3 := []byte("<!-- @license v3 -->\n<svg\n  fill=\"currentColor\"\n  class=\"icons-tabler-filled\"\n>\n  <path d=\"M5 12l-2 0\" />\n</svg>")
	if !sameShape(v2, v3) {
		t.Error("reformatted icon counts as changed")
	}
	if sameShape(v2, icon("M5 12l-3 0")) {
		t.Error("different path counts as same shape")
	}

	polyline := []byte(`<svg viewBox="0 0 24 24" stroke="currentColor" fill="none" stroke-width="2"><polyline points="4 4 20 20"/></svg>`)
	path := []byte(`<svg viewBox="0 0 24 24" stroke="currentColor" fill="none" stroke-width="2"><path d="M4 4L20 20"/></svg>`)
	other := []byte(`<svg viewBox="0 0 24 24" stroke="currentColor" fill="none" stroke-width="2"><path d="M4 20L20 4"/></svg>`)
	if !sameShape(polyline, path) {
		t.Error("the same line as polyline and path counts as changed")
	}
	if sameShape(path, other) {
		t.Error("different lines count as same shape")
	}
}

func TestCompare(t *testing.T) {
	old := map[string][]byte{
		"home":         icon("M1"),
		"edit":         icon("M2"),
		"123":          icon("M3"),
		"cart-x":       icon("M4"),
		"tikto":        icon("M5"),
		"gone":         icon("M6"),
		"ambigous":     icon("M7"),
		"hint":         icon("M8"),
		"help":         icon("M8b"),
		"plus":         icon("M9"),
		"cart-plus":    icon("M9b"), // existed before: found by the fallback to all icons
		"alert-circle": icon("M10"),
		"24-hours":     icon("M11"),
	}
	cur := map[string][]byte{
		"home":                    icon("M1"),
		"edit":                    icon("M2b"),
		"number-123":              icon("M3b"),
		"shopping-cart-x":         icon("M4b"),
		"tiktok":                  icon("M5"),
		"a":                       icon("M7"),
		"b":                       icon("M7"),
		"help":                    icon("M8b"),
		"cart-plus":               icon("M9b"),
		"circle-exclamation-mark": icon("M10"),
		"hours-24":                icon("M11b"),
		"24-hours-x":              icon("M11c"), // would match by name
	}
	upstream := map[string]string{
		"123":         "number-123",
		"24-hours":    "hours-24",
		"gone":        "missing",
		"hint":        "wrong",
		"AlertCircle": "CircleExclamationMark", // identifiers, as Lucide exports them
	}
	d := compare(old, cur, upstream, map[string]string{"hint": "help"})

	if got := strings.Join(d.Changed, " "); got != "edit" {
		t.Errorf("changed = %s", got)
	}
	if got := strings.Join(d.Removed, " "); got != "123 24-hours alert-circle ambigous cart-x gone hint plus tikto" {
		t.Errorf("removed = %s", got)
	}
	want := map[string]rename{
		"123":          {Target: "number-123", Source: sourceUpstream, ShapeChanged: true},
		"alert-circle": {Target: "circle-exclamation-mark", Source: sourceUpstream},
		"24-hours":     {Target: "hours-24", Source: sourceUpstream, ShapeChanged: true},
		"cart-x":       {Target: "shopping-cart-x", Source: sourceName, ShapeChanged: true},
		"tikto":        {Target: "tiktok", Source: sourceShape},
		"hint":         {Target: "help", Source: sourceManual, ShapeChanged: true},
		"plus":         {Target: "cart-plus", Source: sourceName, ShapeChanged: true},
	}
	if len(d.Renames) != len(want) {
		t.Errorf("renames = %v", d.Renames)
	}
	for k, v := range want {
		if d.Renames[k] != v {
			t.Errorf("renames[%q] = %v, want %v", k, d.Renames[k], v)
		}
	}
}

func TestDropAliasFiles(t *testing.T) {
	icons := map[string][]byte{
		"house":          icon("M1"),
		"home":           icon("M1"), // copy under the deprecated name
		"arrow-down-0-1": icon("M2"),
		"arrow-down-01":  icon("M2"), // same identifier as arrow-down-0-1
		"keep":           icon("M3"),
	}
	upstream := map[string]string{
		"Home":          "House",
		"arrow-down-01": "arrow-down-0-1",
		"Unrelated":     "House",
		"KeepMissing":   "Gone",
		"keep":          "keep",
	}
	if n := dropAliasFiles(icons, upstream); n != 2 {
		t.Errorf("dropped %d files", n)
	}
	if icons["home"] != nil || icons["arrow-down-01"] != nil || icons["house"] == nil || icons["arrow-down-0-1"] == nil || icons["keep"] == nil {
		t.Errorf("icons = %v", sortedKeys(icons))
	}
}

func TestApply(t *testing.T) {
	u := testUpdater(t)
	st := &state{
		Version:  "4.1.0",
		Releases: []string{"3.5.0", "4.0.0", "4.1.0"},
		Aliases: []alias{
			{Name: "expired", Target: "home", Since: "3.5.0"},
			{Name: "kept", Target: "home", Since: "4.0.0"},
			{Name: "back", Target: "home", Since: "4.0.0"},
			{Name: "orphan", Target: "vanished", Since: "4.0.0"},
			{Name: "follow", Target: "old-target", Since: "4.0.0"},
		},
	}
	icons := map[string][]byte{"home": icon(), "back": icon(), "new-target": icon(), "renamed": icon()}
	d := iconDiff{
		Removed: []string{"old-target", "vanished", "was"},
		Renames: map[string]rename{"old-target": {Target: "new-target", Source: sourceUpstream}, "was": {Target: "renamed", Source: sourceShape}},
	}
	added, retargeted, dropped := u.apply(st, icons, d)

	var names []string
	for _, a := range st.Aliases {
		names = append(names, a.Name+"→"+a.Target)
	}
	if got := strings.Join(names, " "); got != "follow→new-target kept→home old-target→new-target was→renamed" {
		t.Errorf("aliases = %s", got)
	}
	if len(added) != 2 || added[0].Since != "4.1.0" {
		t.Errorf("added = %v", added)
	}
	if len(retargeted) != 1 || retargeted[0].Name != "follow" {
		t.Errorf("retargeted = %v", retargeted)
	}
	if len(dropped) != 3 {
		t.Errorf("dropped = %v", dropped)
	}
	for _, a := range dropped {
		if a.Name == "expired" && a.Note != "deprecated since Test Icons 3.5.0" {
			t.Errorf("expired note = %q", a.Note)
		}
	}
}

func TestGenerate(t *testing.T) {
	u := testUpdater(t)
	st := &state{
		Version:  "3.52.0",
		Releases: []string{"2.30.0", "3.52.0"},
		Aliases:  []alias{{Name: "123", Target: "number-123", Since: "3.52.0", Source: sourceUpstream}},
	}
	src, err := u.generate(st, map[string]bool{"number-123": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"// Code generated by testupdate from test.json; DO NOT EDIT.",
		"package fyne_test",
		`const TestVersion = "3.52.0"`,
		"// Deprecated: Test Icons 3.52.0 renamed \"123\" to \"number-123\", use IconNumber123. The alias is removed with Test Icons 4.",
		"Icon123 = IconNumber123",
		`"123": "number-123",`,
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("generated file lacks %q:\n%s", want, src)
		}
	}

	same := &state{Version: "1.0.0", Releases: []string{"1.0.0"},
		Aliases: []alias{{Name: "arrow-down-01", Target: "arrow-down-0-1", Since: "1.0.0"}}}
	src, err = u.generate(same, map[string]bool{"arrow-down-0-1": true})
	if err != nil || strings.Contains(string(src), "IconArrowDown01 =") || !strings.Contains(string(src), `"arrow-down-01": "arrow-down-0-1"`) {
		t.Errorf("alias with unchanged identifier: %v\n%s", err, src)
	}

	if _, err := u.generate(st, map[string]bool{}); err == nil {
		t.Error("alias with missing target accepted")
	}
	if _, err := u.generate(st, map[string]bool{"number-123": true, "123": true}); err == nil {
		t.Error("alias shadowing an icon accepted")
	}
}

func TestNotes(t *testing.T) {
	u := testUpdater(t)
	n := notes{
		From: "2.30.0", To: "3.52.0", Total: 3,
		Diff: iconDiff{
			Added:   []string{"fresh", "number-123"},
			Removed: []string{"123", "gone"},
			Changed: []string{"home"},
			Renames: map[string]rename{"123": {Target: "number-123", Source: sourceUpstream, ShapeChanged: true}},
		},
		Aliased: []alias{{Name: "123", Target: "number-123", Source: sourceUpstream}},
		Dropped: []aliasChange{{alias{Name: "old", Target: "home"}, "deprecated since Test Icons 2.30.0"}},
	}
	md := u.markdown(n)
	for _, want := range []string{
		"## Test Icons 3.52.0\n\nUpdated from [2.30.0 to 3.52.0](https://example.com/compare/v2.30.0...v3.52.0), 3 icons in total.",
		"Breaking: 2 `Icon*` variables",
		"| `123` | `number-123` ⚠️ | `Icon123` → `IconNumber123` | Test Icons aliases |",
		"until Test Icons 4",
		"### Removed (1)",
		"`test.json` (source `manual`) and run `go run ./internal/testupdate -offline`",
		"- `gone` (`IconGone`)",
		"- `old` (`IconOld`), use `IconHome`: deprecated since Test Icons 2.30.0",
		"### Added (1)",
		"- `fresh` (`IconFresh`)",
		"### Changed shape (1)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("notes lack %q:\n%s", want, md)
		}
	}
	if shorten(md, "CHANGELOG.md") != md {
		t.Error("short notes were shortened")
	}
}

func TestShorten(t *testing.T) {
	long := "## X\n\n### Added (1)\n\n<details>\n<summary>Show icons</summary>\n\n" + strings.Repeat("- `a`\n", maxNotes) + "\n</details>\n\n### Changed shape (1)\n"
	got := shorten(long, "https://example.com/CHANGELOG.md")
	if len(got) > maxNotes || strings.Contains(got, "<details>") ||
		!strings.Contains(got, "### Added (1)\n\nThe list is in [CHANGELOG.md](https://example.com/CHANGELOG.md).\n") || !strings.Contains(got, "### Changed shape (1)") {
		t.Errorf("shorten = %q", got)
	}
}

func TestChangelog(t *testing.T) {
	u := testUpdater(t)
	file := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := prependChangelog(file, "## Test Icons 3.52.0\n\nold\n"); err != nil {
		t.Fatal(err)
	}
	if err := prependChangelog(file, "## Test Icons 3.53.0\n\nnew\n"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	if string(b) != "# Changelog\n\n## Test Icons 3.53.0\n\nnew\n\n## Test Icons 3.52.0\n\nold\n" {
		t.Errorf("changelog = %q", b)
	}
	crlf := strings.ReplaceAll(string(b), "\n", "\r\n")
	for version, want := range map[string]string{
		"3.53.0": "## Test Icons 3.53.0\n\nnew\n",
		"3.52.0": "## Test Icons 3.52.0\n\nold\n",
	} {
		if got, err := u.changelogSection(lf([]byte(crlf)), version); err != nil || got != want {
			t.Errorf("changelogSection(%s) = %q, %v", version, got, err)
		}
	}
	if _, err := u.changelogSection(string(b), "3.5.0"); err == nil {
		t.Error("missing section accepted")
	}
}
