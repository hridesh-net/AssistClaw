package mempalace

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"go.uber.org/zap"
)

// EnsureOptions configures [Ensure].
type EnsureOptions struct {
	StateDir        string
	BootstrapPython string // host interpreter with the stdlib `venv` module (e.g. python3)
	Progress        io.Writer
	Log             *zap.Logger
}

// Ensure creates (if needed) a dedicated venv under state_dir/mempalace/venv, installs the
// mempalace PyPI package, and runs `mempalace init` once for state_dir/mempalace/world.
// It is idempotent: subsequent calls skip work when the venv and marker file already exist.
func Ensure(ctx context.Context, opts EnsureOptions) error {
	if opts.StateDir == "" {
		return fmt.Errorf("mempalace: state_dir is empty")
	}
	bootstrap := opts.BootstrapPython
	if bootstrap == "" {
		bootstrap = "python3"
	}
	base := ManagedBaseDir(opts.StateDir)
	venvRoot := ManagedVenvRoot(opts.StateDir)
	world := ManagedWorldDir(opts.StateDir)
	marker := WorldInitMarker(opts.StateDir)

	if err := os.MkdirAll(base, 0o755); err != nil {
		return fmt.Errorf("mempalace: mkdir %q: %w", base, err)
	}

	vpy := VenvInterpreter(venvRoot)
	if _, err := os.Stat(vpy); err != nil {
		if opts.Log != nil {
			opts.Log.Info("mempalace: creating venv", zap.String("venv", venvRoot), zap.String("using", bootstrap))
		}
		if err := run(ctx, opts.Progress, bootstrap, "-m", "venv", venvRoot); err != nil {
			return fmt.Errorf("mempalace: create venv with %q: %w", bootstrap, err)
		}
	}
	if _, err := os.Stat(vpy); err != nil {
		return fmt.Errorf("mempalace: venv python missing at %q after venv create", vpy)
	}

	if err := run(ctx, nil, vpy, "-c", "import mempalace"); err != nil {
		if opts.Log != nil {
			opts.Log.Info("mempalace: installing PyPI package (first run may take a while)")
		}
		if err := run(ctx, opts.Progress, vpy, "-m", "pip", "install", "-U", "pip"); err != nil {
			return fmt.Errorf("mempalace: pip upgrade: %w", err)
		}
		if err := run(ctx, opts.Progress, vpy, "-m", "pip", "install", "-U", "mempalace"); err != nil {
			return fmt.Errorf("mempalace: pip install mempalace: %w", err)
		}
	}
	if err := run(ctx, nil, vpy, "-c", "import mempalace"); err != nil {
		return fmt.Errorf("mempalace: import mempalace failed after install: %w", err)
	}

	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	if opts.Log != nil {
		opts.Log.Info("mempalace: initializing world", zap.String("world", world))
	}
	if err := runMempalaceInit(ctx, opts.Progress, venvRoot, world); err != nil {
		return err
	}
	if err := os.WriteFile(marker, []byte("1\n"), 0o644); err != nil {
		return fmt.Errorf("mempalace: write marker: %w", err)
	}
	return nil
}

func runMempalaceInit(ctx context.Context, w io.Writer, venvRoot, world string) error {
	cli := VenvMempalaceCLI(venvRoot)
	if _, err := os.Stat(cli); err == nil {
		if err := run(ctx, w, cli, "init", world); err != nil {
			return fmt.Errorf("mempalace: %q init: %w", cli, err)
		}
		return nil
	}
	vpy := VenvInterpreter(venvRoot)
	if err := run(ctx, w, vpy, "-m", "mempalace", "init", world); err == nil {
		return nil
	}
	if err := run(ctx, w, vpy, "-m", "mempalace.cli", "init", world); err != nil {
		return fmt.Errorf("mempalace: init world %q: %w", world, err)
	}
	return nil
}

func run(ctx context.Context, w io.Writer, name string, arg ...string) error {
	cmd := exec.CommandContext(ctx, name, arg...)
	if w != nil {
		cmd.Stdout = w
		cmd.Stderr = w
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w", filepath.Base(name), arg, err)
	}
	return nil
}
