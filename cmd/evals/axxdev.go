package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

// axxModule is axx's Go module, which a project's axx builds its packs from.
const axxModule = "github.com/nimbusxr/axx"

// devAxxDir holds the axx built from a local checkout, for the base image
// (images/base/axx-dev; empty when the images test a release).
func devAxxDir(evalsDir string) string {
	return filepath.Join(evalsDir, "images", "base", "axx-dev")
}

// prepareDevAxx readies an axx built from the checkout at source for the
// images, as a release would be: a Linux binary of it, at a pseudo-version of
// the next release, and a Go module proxy (a directory) serving the checkout
// as axx's module at that version. axx prepares a project's packs from that
// proxy as a release prepares them from the public one, so a change to axx is
// tested before it is released. It returns the build arguments that replace
// the release's (images/base/axx.env).
func prepareDevAxx(ctx context.Context, evalsDir, source string) ([]string, error) {
	out := devAxxDir(evalsDir)
	if err := os.RemoveAll(out); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	if source == "" {
		return nil, nil
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	files, sum, err := moduleFiles(ctx, source)
	if err != nil {
		return nil, err
	}
	version, when, err := pseudoVersion(ctx, source, sum)
	if err != nil {
		return nil, err
	}
	m := module.Version{Path: axxModule, Version: version}
	dir := filepath.Join(out, "proxy", filepath.FromSlash(axxModule), "@v")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var zb bytes.Buffer
	if err := modzip.Create(&zb, m, files); err != nil {
		return nil, fmt.Errorf("zipping axx's module from %s: %w", source, err)
	}
	gomod, err := os.ReadFile(filepath.Join(source, "go.mod"))
	if err != nil {
		return nil, err
	}
	info, _ := json.Marshal(map[string]string{"Version": version, "Time": when.Format(time.RFC3339)})
	for name, b := range map[string][]byte{
		"list":            []byte(version + "\n"),
		version + ".info": info,
		version + ".mod":  gomod,
		version + ".zip":  zb.Bytes(),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return nil, err
		}
	}
	arch, err := dockerArch(ctx)
	if err != nil {
		return nil, err
	}
	build := exec.CommandContext(ctx, "go", "build", "-trimpath",
		"-ldflags", "-s -w -X "+axxModule+"/internal/version.Version="+strings.TrimPrefix(version, "v"),
		"-o", filepath.Join(out, "axx"), "./cmd/axx")
	build.Dir = source
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return nil, fmt.Errorf("building axx from %s: %w", source, err)
	}
	fmt.Printf("axx %s from %s (%d files in its module)\n", version, source, len(files))
	return []string{
		"AXX_VERSION=" + strings.TrimPrefix(version, "v"),
		"AXX_GOPROXY=file:///opt/axx-dev/proxy,https://proxy.golang.org,direct",
		"AXX_GONOSUMDB=" + axxModule,
	}, nil
}

// moduleFiles lists the files of axx's module in the checkout, as a release's
// module has them: what Git tracks or would (its ignored files left out),
// without nested modules (the examples). sum identifies their contents.
func moduleFiles(ctx context.Context, source string) ([]modzip.File, string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", source, "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, "", fmt.Errorf("listing %s's files: %w", source, err)
	}
	var names []string
	nested := map[string]bool{}
	for _, n := range strings.Split(string(out), "\x00") {
		if n == "" {
			continue
		}
		names = append(names, n)
		if path.Base(n) == "go.mod" && path.Dir(n) != "." {
			nested[path.Dir(n)+"/"] = true
		}
	}
	sort.Strings(names)
	h := sha256.New()
	var files []modzip.File
	for _, n := range names {
		skip := false
		for prefix := range nested {
			if strings.HasPrefix(n, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		f := devFile{root: source, rel: n}
		fi, err := f.Lstat()
		if err != nil || !fi.Mode().IsRegular() {
			continue // deleted in the checkout, or not a file
		}
		b, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(n)))
		if err != nil {
			return nil, "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", n, len(b))
		h.Write(b)
		files = append(files, f)
	}
	return files, hex.EncodeToString(h.Sum(nil)), nil
}

// pseudoVersion is the version of the checkout's module: a pseudo-version of
// the release after its latest tag, at its last commit's time, ending in its
// contents' sum (so the same contents get the same version, and the image
// build's cache holds).
func pseudoVersion(ctx context.Context, source, sum string) (string, time.Time, error) {
	tag, err := exec.CommandContext(ctx, "git", "-C", source, "describe", "--tags", "--abbrev=0", "--match", "v[0-9]*.[0-9]*.[0-9]*").Output()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("finding %s's latest release tag: %w", source, err)
	}
	base := strings.TrimSpace(string(tag))
	parts := strings.SplitN(strings.TrimPrefix(base, "v"), ".", 3)
	patch, err := strconv.Atoi(parts[2])
	if len(parts) != 3 || err != nil {
		return "", time.Time{}, fmt.Errorf("%s: not a release tag", base)
	}
	ct, err := exec.CommandContext(ctx, "git", "-C", source, "log", "-1", "--format=%ct").Output()
	if err != nil {
		return "", time.Time{}, err
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(string(ct)), 10, 64)
	if err != nil {
		return "", time.Time{}, err
	}
	when := time.Unix(secs, 0).UTC()
	return fmt.Sprintf("v%s.%s.%d-0.%s-%s", parts[0], parts[1], patch+1, when.Format("20060102150405"), sum[:12]), when, nil
}

// dockerArch is the architecture of Docker's machine, which the images are
// built for.
func dockerArch(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Arch}}").Output()
	if err != nil {
		return "", fmt.Errorf("asking Docker its architecture: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// devFile is a file of the checkout in axx's module zip.
type devFile struct{ root, rel string }

func (f devFile) Path() string { return f.rel }

func (f devFile) Lstat() (fs.FileInfo, error) {
	return os.Lstat(filepath.Join(f.root, filepath.FromSlash(f.rel)))
}

func (f devFile) Open() (io.ReadCloser, error) {
	return os.Open(filepath.Join(f.root, filepath.FromSlash(f.rel)))
}
