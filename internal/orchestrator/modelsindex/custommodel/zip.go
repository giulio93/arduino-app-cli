// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package custommodel

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/arduino/go-paths-helper"
)

// ErrZipPathTraversal is returned when a ZIP entry's resolved path escapes the
// destination directory (e.g. via "../" components).
var ErrZipPathTraversal = errors.New("zip entry escapes destination directory")

// ExtractZipStream consumes a ZIP archive from rc and unpacks its contents into
// destDir. AI Hub assets arrive as ZIPs whose entries already include the
// expected top-level directory (e.g. "melotts_en-voice_ai-…/…"), so the caller
// typically passes models_repository as destDir and lets the archive lay out
// its own model_directory subtree.
//
// ## Buffering Strategy (Design Rationale)
//
// The stream is buffered to a temporary file inside destDir first (kept on the
// same filesystem to avoid a cross-device move on close), then extracted using
// the standard archive/zip library.
//
// This buffering approach was chosen because:
//
//   1. Go's archive/zip requires io.ReaderAt (random access), which prevents
//      true streaming extraction without a custom ZIP parser.
//
//   2. For UNO Q (64GB eMMC, 4GB RAM), buffering is practical and simpler:
//      - Memory: ZIP size at peak (acceptable for typical model sizes)
//      - Disk: Temp buffer + extracted files (1x total file size overhead)
//      - Simplicity: Uses Go stdlib, no external dependencies
//
//   3. Future enhancement if needed: If supporting devices with <1GB free disk
//      or models >4GB, implement streaming with klauspost/compress/zip or
//      custom ZIP parser. This would reduce memory to ~1MB constant.
//
// See docs/STREAMING-VS-BUFFERING.md for detailed comparison with Python's
// stream-unzip approach and when each strategy is optimal.
//
// Returns the list of files written, so callers can verify what landed where.
func ExtractZipStream(rc io.ReadCloser, destDir *paths.Path) ([]*paths.Path, error) {
	if rc == nil {
		return nil, errors.New("nil ZIP stream")
	}
	defer rc.Close()
	if destDir == nil {
		return nil, errors.New("nil destination directory")
	}
	if err := destDir.MkdirAll(); err != nil {
		return nil, fmt.Errorf("create destination dir: %w", err)
	}

	tmp, err := os.CreateTemp(destDir.String(), "model-download-*.zip")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("buffer ZIP to disk: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("open ZIP: %w", err)
	}
	defer zr.Close()

	cleanDest, err := filepath.Abs(destDir.String())
	if err != nil {
		return nil, fmt.Errorf("resolve destination: %w", err)
	}

	written := make([]*paths.Path, 0, len(zr.File))
	for _, entry := range zr.File {
		out, err := extractZipEntry(entry, cleanDest)
		if err != nil {
			return nil, err
		}
		if out != nil {
			written = append(written, out)
		}
	}
	return written, nil
}

// extractZipEntry writes one ZIP entry under cleanDest, refusing path-traversal
// attempts. Directories return a nil path; files return their on-disk path.
func extractZipEntry(entry *zip.File, cleanDest string) (*paths.Path, error) {
	destPath := filepath.Join(cleanDest, entry.Name)

	rel, err := filepath.Rel(cleanDest, destPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return nil, fmt.Errorf("%w: %s", ErrZipPathTraversal, entry.Name)
	}

	if entry.FileInfo().IsDir() {
		if err := os.MkdirAll(destPath, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", destPath, err)
		}
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir parent of %s: %w", destPath, err)
	}

	src, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("open zip entry %q: %w", entry.Name, err)
	}
	defer src.Close()

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, entry.Mode().Perm())
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", destPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return nil, fmt.Errorf("extract %q: %w", entry.Name, err)
	}

	return paths.New(destPath), nil
}
