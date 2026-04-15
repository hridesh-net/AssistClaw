package mempalace

import (
	"os"
	"path/filepath"
	"runtime"
)

// ManagedBaseDir is the directory under state_dir where AssistClaw keeps MemPalace venv and world.
func ManagedBaseDir(stateDir string) string {
	return filepath.Join(stateDir, "mempalace")
}

// ManagedVenvRoot returns the path to the dedicated Python venv for MemPalace.
func ManagedVenvRoot(stateDir string) string {
	return filepath.Join(ManagedBaseDir(stateDir), "venv")
}

// ManagedWorldDir is the default "world" path passed to `mempalace init` for managed installs.
func ManagedWorldDir(stateDir string) string {
	return filepath.Join(ManagedBaseDir(stateDir), "world")
}

// WorldInitMarker is written after a successful `mempalace init` for the managed world.
func WorldInitMarker(stateDir string) string {
	return filepath.Join(ManagedBaseDir(stateDir), ".world_initialized")
}

// VenvInterpreter returns the python executable inside an existing venv layout.
func VenvInterpreter(venvRoot string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvRoot, "Scripts", "python.exe")
	}
	p := filepath.Join(venvRoot, "bin", "python3")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(venvRoot, "bin", "python")
}

// VenvMempalaceCLI returns the mempalace console script path inside the venv, if present.
func VenvMempalaceCLI(venvRoot string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvRoot, "Scripts", "mempalace.exe")
	}
	return filepath.Join(venvRoot, "bin", "mempalace")
}
