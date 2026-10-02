package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Image names the tasks build on (task Dockerfiles and compose files refer
// to them by these names).
const (
	baseImage     = "axx-evals-base:latest"
	verifierImage = "axx-evals-verifier:latest"
	addressImage  = "axx-evals-address-service:latest"
)

func cmdImages(args []string) error {
	fs := flag.NewFlagSet("images", flag.ExitOnError)
	root := fs.String("root", "", "the evals directory")
	noCache := fs.Bool("no-cache", false, "build without Docker's cache")
	_ = fs.Parse(args)
	dir, err := evalsRoot(*root)
	if err != nil {
		return err
	}
	return buildImages(dir, *noCache)
}

// buildImages builds the evals images from the repository, with the axx
// release images/base/axx.env names.
func buildImages(dir string, noCache bool) error {
	release, err := axxRelease(dir)
	if err != nil {
		return err
	}
	extra := []string{}
	if noCache {
		extra = append(extra, "--no-cache")
	}
	for _, kv := range release {
		extra = append(extra, "--build-arg", kv)
	}
	dockerfile := filepath.Join(dir, "images", "base", "Dockerfile")
	for _, target := range []struct{ name, tag string }{{"base", baseImage}, {"verifier", verifierImage}} {
		args := append([]string{"build", "-f", dockerfile, "--target", target.name, "-t", target.tag}, extra...)
		args = append(args, dir)
		if err := runStreaming("docker", args...); err != nil {
			return fmt.Errorf("building %s: %w", target.tag, err)
		}
	}
	args := []string{"build", "-f", filepath.Join(dir, "images", "address-service", "Dockerfile"), "-t", addressImage}
	if noCache {
		args = append(args, "--no-cache")
	}
	args = append(args, filepath.Join(dir, "app", "wiremock"))
	if err := runStreaming("docker", args...); err != nil {
		return fmt.Errorf("building %s: %w", addressImage, err)
	}
	fmt.Printf("built %s, %s and %s (%s)\n", baseImage, verifierImage, addressImage, strings.Join(release[:1], ""))
	return nil
}

// axxRelease reads images/base/axx.env: the axx release under test and its
// checksums, as NAME=value build arguments.
func axxRelease(dir string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "images", "base", "axx.env"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "=") {
			return nil, fmt.Errorf("images/base/axx.env: %q is not NAME=value", line)
		}
		out = append(out, line)
	}
	if len(out) == 0 || !strings.HasPrefix(out[0], "AXX_VERSION=") {
		return nil, errors.New("images/base/axx.env: AXX_VERSION must come first")
	}
	return out, nil
}

func runStreaming(name string, args ...string) error {
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func cmdPacks(args []string) error {
	fs := flag.NewFlagSet("packs", flag.ExitOnError)
	_ = fs.Parse(args)
	packs, version, err := imagePacks()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(packs))
	for p := range packs {
		names = append(names, p)
	}
	sort.Strings(names)
	fmt.Printf("axx %s: %s\n", version, strings.Join(names, ", "))
	return nil
}

// imagePacks lists the packs the axx in the evals image publishes (`axx pack
// list`: the ones a project can list in axx-packs.yaml), and its version.
func imagePacks() (map[string]bool, string, error) {
	out, err := exec.CommandContext(context.Background(), "docker", "run", "--rm", "-w", "/opt/evals/workspace", "--entrypoint", "axx", baseImage, "pack", "list", "--json").Output()
	if err != nil {
		return nil, "", fmt.Errorf("listing the packs in %s (run `evals images` first): %w", baseImage, err)
	}
	var env struct {
		Data struct {
			Packs []struct {
				Pack string `json:"pack"`
				Kind string `json:"kind"`
			} `json:"packs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, "", err
	}
	packs := map[string]bool{"core": true}
	for _, p := range env.Data.Packs {
		if p.Kind == "axx" {
			packs[p.Pack] = true
		}
	}
	vout, _ := exec.CommandContext(context.Background(), "docker", "run", "--rm", "--entrypoint", "axx", baseImage, "version", "--json").Output()
	var venv struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(bytes.TrimSpace(vout), &venv)
	return packs, venv.Data.Version, nil
}
