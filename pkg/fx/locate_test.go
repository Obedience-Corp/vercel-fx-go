package fx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocateBinaryPrefersInstallDir(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fx")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("FX_INSTALL_DIR", dir)
	got, err := LocateBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got != binary {
		t.Fatalf("located %q, want %q", got, binary)
	}
}

func TestLocateBinaryReportsTransportWhenMissing(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	t.Setenv("FX_INSTALL_DIR", empty)
	t.Setenv("HOME", empty)
	if _, err := os.Stat("/usr/bin/fx"); err == nil {
		t.Skip("this machine has a system-wide fx install")
	}
	if _, err := os.Stat("/usr/local/bin/fx"); err == nil {
		t.Skip("this machine has a system-wide fx install")
	}
	if _, err := os.Stat("/opt/homebrew/bin/fx"); err == nil {
		t.Skip("this machine has a homebrew fx install")
	}
	_, err := LocateBinary()
	requireFxError(t, err, KindTransport)
}

func TestLocateBinaryIgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "fx"), 0o755); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	t.Setenv("FX_INSTALL_DIR", dir)
	t.Setenv("HOME", empty)
	if _, err := os.Stat("/usr/bin/fx"); err == nil {
		t.Skip("this machine has a system-wide fx install")
	}
	if got, err := LocateBinary(); err == nil && got == filepath.Join(dir, "fx") {
		t.Fatal("a directory named fx must not be selected")
	}
}
