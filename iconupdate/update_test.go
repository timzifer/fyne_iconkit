package iconupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const svg = `<svg stroke="currentColor"></svg>`

func testUpdater(t *testing.T) *updater {
	dir := t.TempDir()
	return &updater{
		src: Source{
			Name:         "Test Icons",
			URL:          "https://example.com/icons",
			Package:      "test-icons",
			Dirs:         map[string]string{"package/icons/outline/": "", "package/icons/filled/": "-filled"},
			License:      "package/LICENSE",
			CompareURL:   func(from, to string) string { return "https://example.com/compare/v" + from + "...v" + to },
			Command:      "go run ./internal/testupdate",
			GoPackage:    "fyne_test",
			VersionConst: "TestVersion",
			StateFile:    "test.json",
			Out:          "test_gen.go",
			LicenseFile:  "LICENSE-test",
		},
		o: options{
			dir:       filepath.Join(dir, "icons"),
			readme:    filepath.Join(dir, "README.md"),
			stateFile: filepath.Join(dir, "test.json"),
			changelog: filepath.Join(dir, "CHANGELOG.md"),
			pkg:       "fyne_test",
		},
	}
}

func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtract(t *testing.T) {
	u := testUpdater(t)
	u.src.Files = []string{"package/extra.json"}
	u.src.Clean = func(b []byte) []byte { return bytes.TrimPrefix(b, []byte("<!-- v1 -->")) }
	pkg, err := u.extract(tarball(t, map[string]string{
		"package/LICENSE":                   "MIT",
		"package/extra.json":                "{}",
		"package/icons/outline/home.svg":    "<!-- v1 -->" + svg,
		"package/icons/filled/home.svg":     `<svg fill="currentColor"></svg>`,
		"package/categories/outline/x.svg":  svg,
		"package/icons/outline/readme.txt":  "ignored",
		"package/icons/outline/sub/sub.svg": svg,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if string(pkg.license) != "MIT" || string(pkg.files["package/extra.json"]) != "{}" {
		t.Errorf("license = %q, files = %v", pkg.license, pkg.files)
	}
	if len(pkg.icons) != 2 || string(pkg.icons["home"]) != svg || pkg.icons["home-filled"] == nil {
		t.Errorf("icons = %q", pkg.icons)
	}
}

func TestExtractRejects(t *testing.T) {
	u := testUpdater(t)
	for name, files := range map[string]map[string]string{
		"collision": {
			"package/LICENSE":                       "MIT",
			"package/icons/outline/home-filled.svg": svg,
			"package/icons/filled/home.svg":         svg,
		},
		"bad name":      {"package/LICENSE": "MIT", "package/icons/outline/Home.svg": svg},
		"not stainable": {"package/LICENSE": "MIT", "package/icons/outline/home.svg": `<svg stroke="#000"></svg>`},
		"no icons":      {"package/LICENSE": "MIT"},
		"no license":    {"package/icons/outline/home.svg": svg},
	} {
		if _, err := u.extract(tarball(t, files)); err == nil {
			t.Errorf("%s: extract succeeded", name)
		}
	}
}

func TestVerify(t *testing.T) {
	data := []byte("data")
	sum := sha512.Sum512(data)
	good := "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
	if err := verify(data, good); err != nil {
		t.Error(err)
	}
	if err := verify([]byte("other"), good); err == nil {
		t.Error("verify accepted wrong data")
	}
	if err := verify(data, "sha1-abc"); err == nil {
		t.Error("verify accepted unsupported algorithm")
	}
}

func TestWriteIcons(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"keep.svg": "old", "same.svg": "same", "gone.svg": "x", "other.txt": "x"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := writeIcons(dir, map[string][]byte{"keep": []byte("new"), "same": []byte("same"), "new": []byte("new")})
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if got := strings.Join(names, " "); got != "keep.svg new.svg other.txt same.svg" {
		t.Errorf("files = %s", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "keep.svg")); string(b) != "new" {
		t.Errorf("keep.svg = %q", b)
	}
}

func TestReadme(t *testing.T) {
	u := testUpdater(t)
	if err := os.WriteFile(u.o.readme, []byte("[Test Icons](https://example.com/icons) (v2.30.0) for"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := u.updateReadme("3.52.0"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(u.o.readme); string(b) != "[Test Icons](https://example.com/icons) (v3.52.0) for" {
		t.Errorf("README = %q", b)
	}
	if v, err := u.readmeVersion(); err != nil || v != "3.52.0" {
		t.Errorf("readmeVersion = %q, %v", v, err)
	}
	if err := os.WriteFile(u.o.readme, []byte("no version"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := u.updateReadme("3.52.0"); err == nil {
		t.Error("updateReadme accepted README without version")
	}
}

func TestLoadState(t *testing.T) {
	u := testUpdater(t)
	if err := os.WriteFile(u.o.readme, []byte("[Test Icons](https://example.com/icons) (v1.2.0)"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := u.loadState()
	if err != nil || st.Version != "1.2.0" || len(st.Releases) != 1 {
		t.Fatalf("bootstrap = %+v, %v", st, err)
	}
	st.Aliases = []alias{{Name: "a", Target: "b", Since: "1.2.0", Source: sourceManual}}
	if err := st.save(u.o.stateFile); err != nil {
		t.Fatal(err)
	}
	if st, err := u.loadState(); err != nil || len(st.Aliases) != 1 {
		t.Errorf("reload = %+v, %v", st, err)
	}
	if err := os.WriteFile(u.o.stateFile, []byte(`{"version": "2.0.0", "releases": ["1.0.0"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := u.loadState(); err == nil {
		t.Error("version missing from releases accepted")
	}
}
