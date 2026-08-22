package fx

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// LocateBinary finds the fx binary on PATH or in the standard install locations.
func LocateBinary() (string, error) {
	if p, err := exec.LookPath("fx"); err == nil {
		return p, nil
	}
	for _, candidate := range binaryCandidates() {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate, nil
	}
	return "", &Error{Kind: KindTransport, Message: "fx binary not found on PATH or in standard install locations"}
}

func binaryCandidates() []string {
	candidates := make([]string, 0, 6)
	if dir := os.Getenv("FX_INSTALL_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "fx"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", "fx"))
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/opt/homebrew/bin/fx")
	}
	return append(candidates, "/usr/local/bin/fx", "/usr/bin/fx")
}
