// SPDX-License-Identifier: AGPL-3.0-only

package bootstrap

import (
	"path/filepath"
	"testing"
)

func TestLoad_NeverConfigured(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load(missing file): %v", err)
	}
	if cfg != nil {
		t.Errorf("Load(missing file) = %+v, want nil", cfg)
	}
	if cfg.Configured() {
		t.Error("nil Config reports Configured() = true")
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "bootstrap.json")
	want := &Config{
		Mode:    ModeStandalone,
		DataDir: "/var/lib/ovl-vessel/data",
		Enrollment: Enrollment{
			OfficeURL: "https://office.example.com",
			Code:      "ABC123",
			Submitted: true,
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *got != *want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
	if !got.Configured() {
		t.Error("Configured() = false for a config with a DataDir set")
	}
}

func TestConfig_Configured(t *testing.T) {
	if (&Config{}).Configured() {
		t.Error("Configured() = true for an empty DataDir")
	}
	if !(&Config{DataDir: "/data"}).Configured() {
		t.Error("Configured() = false for a non-empty DataDir")
	}
}

func TestMode_Valid(t *testing.T) {
	for _, m := range []Mode{ModeStandalone, ModeServer} {
		if !m.Valid() {
			t.Errorf("Mode(%q).Valid() = false", m)
		}
	}
	if Mode("offshore").Valid() {
		t.Error(`Mode("offshore").Valid() = true, want false`)
	}
}
