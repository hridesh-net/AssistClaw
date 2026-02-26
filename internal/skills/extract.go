package skills

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractTarGz extracts the first top-level directory from a .tar.gz archive into destDir.
// This is used when downloading community skills from GitHub (archive/refs/heads/main.tar.gz).
func extractTarGz(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var topDir string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Determine top-level dir (GitHub adds repo-branch/ prefix)
		parts := strings.SplitN(header.Name, "/", 2)
		if topDir == "" && len(parts) > 0 {
			topDir = parts[0]
		}

		// Strip top-level dir from path
		rel := header.Name
		if topDir != "" && strings.HasPrefix(header.Name, topDir+"/") {
			rel = strings.TrimPrefix(header.Name, topDir+"/")
		}
		if rel == "" {
			continue
		}

		target := filepath.Join(destDir, rel)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
