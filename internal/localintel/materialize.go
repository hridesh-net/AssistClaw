package localintel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const embeddedModelFileName = "gemma-4-e2b-it.gguf"

// MaterializeGGUF writes embedded weights to cacheDir if non-empty, returning the absolute path.
// If the target file already exists with the same SHA-256 prefix in its name, it is reused.
func MaterializeGGUF(cacheDir string) (path string, err error) {
	if len(Gemma4E2BGGUF) == 0 {
		return "", ErrEmbeddedWeightsEmpty
	}
	sum := sha256.Sum256(Gemma4E2BGGUF)
	id := hex.EncodeToString(sum[:8])
	dir := filepath.Join(cacheDir, "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("localintel: mkdir models cache: %w", err)
	}
	dst := filepath.Join(dir, fmt.Sprintf("gemma-4-e2b-it.%s.gguf", id))
	if st, err := os.Stat(dst); err == nil && st.Size() == int64(len(Gemma4E2BGGUF)) {
		return dst, nil
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, Gemma4E2BGGUF, 0o644); err != nil {
		return "", fmt.Errorf("localintel: write temp gguf: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("localintel: rename gguf: %w", err)
	}
	return dst, nil
}

// ResolveGGUFPath returns a filesystem path to the model: GGUFPath if set, else materialized embed.
func ResolveGGUFPath(cacheDir, ggufPath string) (string, error) {
	if ggufPath != "" {
		st, err := os.Stat(ggufPath)
		if err != nil {
			return "", fmt.Errorf("localintel: stat gguf: %w", err)
		}
		if st.IsDir() {
			return "", fmt.Errorf("localintel: gguf path is a directory: %s", ggufPath)
		}
		return ggufPath, nil
	}
	if len(Gemma4E2BGGUF) == 0 {
		return "", ErrNoWeights
	}
	return MaterializeGGUF(cacheDir)
}

// EmbeddedModelFileName returns the filename expected under embedded/ for the assistclaw_embedlocalgemma tag.
func EmbeddedModelFileName() string { return embeddedModelFileName }
