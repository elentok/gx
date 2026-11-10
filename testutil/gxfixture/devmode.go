package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/testutil/vitefake"
)

// devModeControl is the fake pnpm's starting behaviour for each shape that
// turns Dev mode on; the flow moves it afterwards by rewriting the file.
var devModeControl = map[string]vitefake.Control{
	"installing": {Install: "hold"},
	"starting":   {Vite: "hold"},
}

// setupDevMode lays out the dev-mode scenario: a Dev checkout (none for
// no-checkout), the fake pnpm first on PATH with its control file, and the
// config that points the server at them. installing is the one shape with a
// stale pair (node_modules older than pnpm-lock.yaml), so the server installs.
func (f *fixture) setupDevMode(shape string) (config []string, err error) {
	dir := filepath.Join(f.root, "devmode")
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return nil, err
	}
	if _, err := binary("", filepath.Join(bin, "pnpm"), "github.com/elentok/gx/testutil/vitefake/cmd/pnpm"); err != nil {
		return nil, err
	}
	f.prependPath(bin)
	f.devControl = filepath.Join(dir, "vitefake.json")
	if err := vitefake.WriteControl(f.devControl, devModeControl[shape]); err != nil {
		return nil, err
	}
	f.env = append(f.env, vitefake.ControlEnv+"="+f.devControl)

	if shape == "no-checkout" {
		return []string{"server.web.dev-frontend=true"}, nil
	}
	checkout := filepath.Join(dir, "checkout")
	webui := filepath.Join(checkout, "webui")
	if err := os.MkdirAll(filepath.Join(webui, "node_modules"), 0o755); err != nil {
		return nil, err
	}
	for name, body := range map[string]string{"package.json": "{}\n", "pnpm-lock.yaml": "lockfileVersion: '9.0'\n"} {
		if err := os.WriteFile(filepath.Join(webui, name), []byte(body), 0o644); err != nil {
			return nil, err
		}
	}
	lockAge, modulesAge := -time.Hour, time.Duration(0)
	if shape == "installing" {
		lockAge, modulesAge = 0, -time.Hour
	}
	now := time.Now()
	for name, age := range map[string]time.Duration{"pnpm-lock.yaml": lockAge, "node_modules": modulesAge} {
		at := now.Add(age)
		if err := os.Chtimes(filepath.Join(webui, name), at, at); err != nil {
			return nil, fmt.Errorf("age %s: %w", name, err)
		}
	}
	config = []string{"server.web.dev-dir=" + checkout}
	if shape != "off" {
		config = append(config, "server.web.dev-frontend=true")
	}
	return config, nil
}
