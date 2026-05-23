// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package custommodel

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildZip returns an in-memory ZIP with the given path→content entries.
func buildZip(t *testing.T, entries map[string]string) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return io.NopCloser(&buf)
}

func TestExtractZipStream_FlatAndNested(t *testing.T) {
	dest := paths.New(t.TempDir())

	rc := buildZip(t, map[string]string{
		"top.txt":          "alpha",
		"sub/dir/file.bin": "beta",
		"sub/other.txt":    "gamma",
	})

	written, err := ExtractZipStream(rc, dest)
	require.NoError(t, err)
	require.Len(t, written, 3)

	got, err := dest.Join("top.txt").ReadFile()
	require.NoError(t, err)
	assert.Equal(t, "alpha", string(got))

	got, err = dest.Join("sub", "dir", "file.bin").ReadFile()
	require.NoError(t, err)
	assert.Equal(t, "beta", string(got))

	got, err = dest.Join("sub", "other.txt").ReadFile()
	require.NoError(t, err)
	assert.Equal(t, "gamma", string(got))
}

func TestExtractZipStream_RejectsPathTraversal(t *testing.T) {
	dest := paths.New(t.TempDir())

	rc := buildZip(t, map[string]string{
		"../escape.txt": "evil",
	})

	_, err := ExtractZipStream(rc, dest)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrZipPathTraversal),
		"want ErrZipPathTraversal, got %v", err)

	// The escaped file must NOT have been created on disk.
	assert.False(t, dest.Parent().Join("escape.txt").Exist(),
		"path-traversal entry should not be written")
}

func TestExtractZipStream_NilStream(t *testing.T) {
	_, err := ExtractZipStream(nil, paths.New(t.TempDir()))
	require.Error(t, err)
}

func TestExtractZipStream_NilDest(t *testing.T) {
	rc := buildZip(t, map[string]string{"a.txt": "x"})
	_, err := ExtractZipStream(rc, nil)
	require.Error(t, err)
}

func TestExtractZipStream_NonZipPayload(t *testing.T) {
	dest := paths.New(t.TempDir())
	rc := io.NopCloser(bytes.NewReader([]byte("definitely not a zip")))
	_, err := ExtractZipStream(rc, dest)
	require.Error(t, err)
}

func TestExtractZipStream_TopLevelDirIsPreserved(t *testing.T) {
	// AI Hub asset ZIPs typically wrap their files in a single top-level
	// directory whose name matches the `model_directory` deployment variable.
	// The extractor must preserve that wrapping so downstream code can locate
	// the model under models_repository/model_directory.
	dest := paths.New(t.TempDir())

	rc := buildZip(t, map[string]string{
		"melotts_en-voice_ai-mixed_with_float-qualcomm_qcs8275/":           "",
		"melotts_en-voice_ai-mixed_with_float-qualcomm_qcs8275/model.bin":  "weights",
		"melotts_en-voice_ai-mixed_with_float-qualcomm_qcs8275/config.txt": "cfg",
	})

	_, err := ExtractZipStream(rc, dest)
	require.NoError(t, err)

	wrap := dest.Join("melotts_en-voice_ai-mixed_with_float-qualcomm_qcs8275")
	exists, _ := wrap.IsDirCheck()
	require.True(t, exists, "top-level wrapping directory should exist")
	assert.True(t, wrap.Join("model.bin").Exist())
	assert.True(t, wrap.Join("config.txt").Exist())
}
