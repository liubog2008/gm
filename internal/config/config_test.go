package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesFlagFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load("/flag/base", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseDir != "/flag/base" {
		t.Fatalf("BaseDir = %q, want %q", cfg.BaseDir, "/flag/base")
	}
}

func TestLoadUsesDefaultYAMLConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".config", "gm", "config.yaml"), "base_dir: ./repos\n")

	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(wd, "repos")
	if cfg.BaseDir != want {
		t.Fatalf("BaseDir = %q, want %q", cfg.BaseDir, want)
	}
}

func TestLoadUsesExplicitConfigPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "gm.yaml")
	writeFile(t, configPath, "base: /yaml/base\n")

	cfg, err := Load("", configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseDir != "/yaml/base" {
		t.Fatalf("BaseDir = %q, want %q", cfg.BaseDir, "/yaml/base")
	}
}

func TestLoadMissingConfigReportsDefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := Load("", "")
	if err == nil {
		t.Fatal("expected error")
	}

	want := filepath.Join(home, ".config", "gm", "config.yaml")
	if err.Error() != "missing base dir: set --base or base_dir in "+want {
		t.Fatalf("error = %q", err)
	}
}

func TestLoadInvalidYAMLReturnsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "gm.yaml")
	writeFile(t, configPath, "base_dir: [\n")

	if _, err := Load("", configPath); err == nil {
		t.Fatal("expected error")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
