// SPDX-License-Identifier: AGPL-3.0-only

// Package bootstrap holds the tiny pointer file ovl-vessel reads before
// it can open anything else: which data directory the SQLite store
// lives in, and the operating mode (architecture 9.1/9.2). It has to
// live outside that data directory — the vessel can't know where the
// database is until this file tells it — so it always lives at a fixed,
// well-known OS-appropriate location instead.
package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Mode is the vessel's operating mode (architecture 9.1). Only
// ModeStandalone is functional in Phase 2; ModeServer is accepted and
// stored but has no behavioral effect yet (LAN binding is Phase 4).
type Mode string

const (
	ModeStandalone Mode = "standalone"
	ModeServer     Mode = "server"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	return m == ModeStandalone || m == ModeServer
}

// Enrollment is the office URL/code pair from architecture 9.2's
// wizard step 2. Submitted is set once the operator has gone through
// that step (whether they entered real values or explicitly skipped
// it) — nothing here is validated against a real office, since no
// office/sync connection exists yet (Phase 4). It exists so the UI can
// show a persistent "Not enrolled" reminder per the design handoff's
// explicit requirement for the offline-install path.
type Enrollment struct {
	OfficeURL string `json:"officeURL"`
	Code      string `json:"code"`
	Submitted bool   `json:"submitted"`
}

// Config is the bootstrap file's contents.
type Config struct {
	Mode       Mode       `json:"mode"`
	DataDir    string     `json:"dataDir"`
	Enrollment Enrollment `json:"enrollment"`
}

// Configured reports whether the operator has completed wizard step 1
// (mode + data directory) yet.
func (c *Config) Configured() bool {
	return c != nil && c.DataDir != ""
}

const appDirName = "ovl-vessel"

// DefaultPath returns the OS-appropriate location for the bootstrap
// file: <UserConfigDir>/ovl-vessel/bootstrap.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("bootstrap: resolve user config dir: %w", err)
	}
	return filepath.Join(dir, appDirName, "bootstrap.json"), nil
}

// DefaultDataDir suggests where the SQLite store and attachments should
// live, for the wizard to pre-fill — <UserConfigDir>/ovl-vessel/data.
// The operator can type a different path; Go's standard library has no
// portable "user data dir" distinct from the config dir, so this keeps
// both under the same app folder rather than guessing at
// Documents/Application-Support-style conventions per OS.
func DefaultDataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("bootstrap: resolve user config dir: %w", err)
	}
	return filepath.Join(dir, appDirName, "data"), nil
}

// Load reads and parses the bootstrap file at path. A missing file is
// not an error: it returns (nil, nil), meaning "never configured."
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the operator's own -config flag/OS default, never remote input
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bootstrap: read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("bootstrap: parse %s: %w", path, err)
	}
	return &cfg, nil
}

// Save writes cfg to path, creating the parent directory if needed.
// Writes to a temp file in the same directory and renames into place so
// a crash mid-write can't leave a truncated bootstrap file.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("bootstrap: create %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("bootstrap: marshal config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "bootstrap-*.json")
	if err != nil {
		return fmt.Errorf("bootstrap: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once renamed below

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("bootstrap: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("bootstrap: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("bootstrap: save %s: %w", path, err)
	}
	return nil
}
