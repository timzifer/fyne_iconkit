package iconupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
)

const registry = "https://registry.npmjs.org/"

// ErrNotFound is returned by Fetch for HTTP 404 responses.
var ErrNotFound = errors.New("not found")

// Fetch returns the body of an HTTP GET request; 404 yields an error
// wrapping ErrNotFound.
func Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "fyne_iconkit-iconupdate")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return io.ReadAll(resp.Body)
	case http.StatusNotFound:
		return nil, fmt.Errorf("GET %s: %w", rawURL, ErrNotFound)
	default:
		return nil, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
}

// release is the subset of the npm registry's version document we need.
type release struct {
	Version string `json:"version"`
	Dist    struct {
		Tarball   string `json:"tarball"`
		Integrity string `json:"integrity"`
	} `json:"dist"`
}

func fetchRelease(ctx context.Context, pkg, version string) (*release, error) {
	body, err := Fetch(ctx, registry+pkg+"/"+url.PathEscape(version))
	if err != nil {
		return nil, err
	}
	var rel release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("decode release %q: %w", version, err)
	}
	if rel.Version == "" || rel.Dist.Tarball == "" {
		return nil, fmt.Errorf("release %q: no version or tarball in registry response", version)
	}
	return &rel, nil
}

func download(ctx context.Context, tarball string) ([]byte, error) {
	u, err := url.Parse(tarball)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("refusing non-https tarball URL %q", tarball)
	}
	return Fetch(ctx, tarball)
}

// verify checks data against an npm Subresource Integrity string
// ("sha512-<base64>").
func verify(data []byte, integrity string) error {
	want, ok := strings.CutPrefix(integrity, "sha512-")
	if !ok {
		return fmt.Errorf("unsupported integrity %q", integrity)
	}
	sum := sha512.Sum512(data)
	if got := base64.StdEncoding.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("tarball integrity mismatch: got sha512-%s, want %s", got, integrity)
	}
	return nil
}

type npmPackage struct {
	icons   map[string][]byte // name without ".svg" → SVG content
	license []byte
	files   map[string][]byte // Source.Files
}

var validName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// extract reads the icons, the license and the requested files from an npm
// package tarball.
func (u *updater) extract(tgz []byte) (*npmPackage, error) {
	zr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, err
	}
	pkg := &npmPackage{icons: map[string][]byte{}, files: map[string][]byte{}}
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		dir, file := path.Split(hdr.Name)
		suffix, isIcon := u.src.Dirs[dir]
		isIcon = isIcon && strings.HasSuffix(file, ".svg")
		switch {
		case hdr.Name == u.src.License:
			if pkg.license, err = io.ReadAll(tr); err != nil {
				return nil, err
			}
			continue
		case slices.Contains(u.src.Files, hdr.Name):
			if pkg.files[hdr.Name], err = io.ReadAll(tr); err != nil {
				return nil, err
			}
			continue
		case !isIcon:
			continue
		}

		name := strings.TrimSuffix(file, ".svg") + suffix
		if !validName.MatchString(name) {
			return nil, fmt.Errorf("%s: unexpected icon name %q", hdr.Name, name)
		}
		if _, dup := pkg.icons[name]; dup {
			return nil, fmt.Errorf("%s: icon %q exists twice", hdr.Name, name)
		}
		src, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		if u.src.Clean != nil {
			src = u.src.Clean(src)
		}
		src = normalize(src)
		if !bytes.Contains(src, []byte("currentColor")) {
			return nil, fmt.Errorf("%s: icon does not use currentColor and cannot be stained", hdr.Name)
		}
		pkg.icons[name] = src
	}

	if len(pkg.icons) == 0 {
		return nil, errors.New("tarball contains no icons")
	}
	if u.src.License != "" && pkg.license == nil {
		return nil, fmt.Errorf("tarball contains no %s", u.src.License)
	}
	for _, f := range u.src.Files {
		if pkg.files[f] == nil {
			return nil, fmt.Errorf("tarball contains no %s", f)
		}
	}
	return pkg, nil
}
