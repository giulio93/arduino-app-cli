// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package modelsindex

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

func TestParseInfoLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		size    uint64
		errText string
		isErr   bool
	}{
		{"HF stat: bytes win over MiB", `{"event":"stat","size_bytes":1000,"size_mb":5}`, 1000, "", false},
		{"zero bytes falls back to MiB", `{"event":"stat","size_bytes":0,"size_mb":46}`, mibToBytes(46), "", false},
		{"AI Hub stat: MiB only", `{"event":"stat","size_mb":46}`, mibToBytes(46), "", false},
		{"null size is unknown", `{"event":"stat","size_mb":null}`, 0, "", false},
		{"-1 is unknown, not a parse error", `{"event":"stat","size_bytes":-1}`, 0, "", false},
		{"error with text", `{"event":"error","description":"gated"}`, 0, "gated", true},
		{"error without text is still an error", `{"event":"error"}`, 0, "", true},
		{"other events are neither", `{"event":"info","description":"x"}`, 0, "", false},
		{"not JSON", `Downloading...`, 0, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			size, text, isErr := parseInfoLine(tc.line)
			assert.Equal(t, tc.size, size)
			assert.Equal(t, tc.errText, text)
			assert.Equal(t, tc.isErr, isErr)
		})
	}
}

// TestClassifyInfoError uses the sentences hf_downloader.py prints today, whole, so a
// rewording on the container side shows up here as a row falling to ErrInfoFailed.
func TestClassifyInfoError(t *testing.T) {
	tests := []struct {
		text string
		want error
	}{
		{"Invalid Hugging Face URL: foo\nnot a link\nuse https://huggingface.co/...", ErrBadModelURL},
		{"Hugging Face model repository 'a/b' does not exist, or is not public. Only public model repositories can be downloaded.", ErrModelNotFound},
		{"Hugging Face repository 'a/b' is gated: its files are only served after its conditions are accepted.", ErrModelForbidden},
		{"Hugging Face repository 'a/b' is private. Only public model repositories can be downloaded.", ErrModelForbidden},
		{"Hugging Face repository 'a/b' has been disabled by its authors.", ErrModelGone},
		{"Revision 'x' does not exist in Hugging Face repository 'a/b'.", ErrModelNotFound},
		{"File 'm.gguf' does not exist in Hugging Face repository 'a/b', which contains no GGUF files at all.", ErrModelNotFound},
		{"No file matching '*Q4_0*.gguf' found in repository 'a/b'.", ErrModelNotFound},
		{"Cannot download model. Not supported quantization: Q2_K.", ErrUnsupportedModel},
		{"Could not verify Hugging Face repository 'a/b': connection refused", ErrHubUnreachable},
		{"something nobody has seen before", ErrInfoFailed},
	}
	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			err := classifyInfoError(tc.text)
			require.ErrorIs(t, err, tc.want)
			assert.Contains(t, err.Error(), tc.text, "the container's words reach the user")
		})
	}
}

// TestPrecheck drives precheck against a fake container: check answers first, then info.
func TestPrecheck(t *testing.T) {
	const url = "llamacpp:org/repo:Q4_0"
	plat := platform.Platform{BoardName: "ventunoq"}
	// entries are what the index lists: piper only by default.
	newIndex := func(t *testing.T, info, check string, checkExit int, entries ...string) (*ModelsIndex, *fakeDockerClient, *atomic.Int64) {
		t.Helper()
		var infos atomic.Int64
		cli := newFakeDockerClient(func(_ string, cmd []string) (string, int) {
			switch {
			case len(cmd) > 0 && strings.Contains(cmd[0], "_info.sh"):
				infos.Add(1)
				return info + "\n", 0
			case len(cmd) > 0 && strings.Contains(cmd[0], "_checker.sh"):
				return check + "\n", checkExit
			}
			t.Errorf("unexpected container %v", cmd)
			return "", 1
		})
		modelsDir := paths.New(t.TempDir())
		writeIndex(t, modelsDir, append([]string{piperEntry}, entries...)...)
		dir := paths.New("testdata/with-handlers")
		idx, err := Load(plat, dir, modelsDir, dir.Join("custom-models"), cli, config.Configuration{})
		require.NoError(t, err)
		return idx, cli, &infos
	}
	const notInstalled = `{"event":"error","description":"Model does not exist","downloading":false,"status":"not_installed"}`

	t.Run("a model nobody has: proceed, with the size info reported", func(t *testing.T) {
		idx, cli, _ := newIndex(t, `{"event":"stat","size_bytes":1024}`, notInstalled, 1)
		res, err := idx.PrecheckDownload(t.Context(), cli, url, "", plat)
		require.NoError(t, err)
		assert.Equal(t, PrecheckResult{SizeBytes: 1024}, res)
	})

	t.Run("a download or delete running on it: ErrInstallInProgress, the hub not asked", func(t *testing.T) {
		idx, cli, infos := newIndex(t, `{"event":"stat","size_bytes":1024}`,
			`{"event":"info","description":"Model downloading","downloading":true,"status":"in_progress"}`, 0)
		_, err := idx.PrecheckDownload(t.Context(), cli, url, "", plat)
		require.ErrorIs(t, err, ErrInstallInProgress)
		assert.Zero(t, infos.Load())
	})

	t.Run("info fails: classified", func(t *testing.T) {
		idx, cli, _ := newIndex(t, `{"event":"error","description":"Hugging Face repository 'org/repo' is gated: no"}`, notInstalled, 1)
		_, err := idx.PrecheckDownload(t.Context(), cli, url, "", plat)
		require.ErrorIs(t, err, ErrModelForbidden)
	})

	t.Run("info fails for a curated model: always ErrInfoFailed", func(t *testing.T) {
		idx, cli, _ := newIndex(t, `{"event":"error","description":"Hugging Face repository 'org/repo' is gated: no"}`, notInstalled, 1)
		_, err := idx.precheck(t.Context(), cli, userHFModel(url, "", plat), "curated-key", false, plat)
		require.ErrorIs(t, err, ErrInfoFailed)
		assert.NotErrorIs(t, err, ErrModelForbidden)
	})

	const exists = `{"event":"info","description":"Model exists","downloading":false,"status":"installed"}`

	t.Run("on disk and recorded from this link: Installed", func(t *testing.T) {
		// downloadedEntry's record names this very link as its model_url.
		idx, cli, _ := newIndex(t, `{"event":"stat","size_bytes":1024}`, exists, 0, downloadedEntry)
		res, err := idx.PrecheckDownload(t.Context(), cli, url, "", plat)
		require.NoError(t, err)
		assert.True(t, res.Installed)
	})

	t.Run("on disk but no record of it: a leftover, download again", func(t *testing.T) {
		// Seen on a board: a download stopped after its file landed, before its record.
		// check finds the file, the index does not list it, so a 409 "already
		// installed" would refuse a model nobody can see or delete.
		idx, cli, _ := newIndex(t, `{"event":"stat","size_bytes":1024}`, exists, 0)
		res, err := idx.PrecheckDownload(t.Context(), cli, url, "", plat)
		require.NoError(t, err)
		assert.False(t, res.Installed)
	})

	t.Run("a curated install is not confirmed against the index", func(t *testing.T) {
		// Curated models are listed from their declaration, record or not: check is enough.
		idx, cli, _ := newIndex(t, `{"event":"stat","size_bytes":1024}`, exists, 0)
		res, err := idx.precheck(t.Context(), cli, userHFModel(url, "", plat), "curated-key", false, plat)
		require.NoError(t, err)
		assert.True(t, res.Installed)
	})

	t.Run("a download marker with the lock free is a leftover: proceed", func(t *testing.T) {
		idx, cli, _ := newIndex(t, `{"event":"stat","size_bytes":1024}`,
			`{"event":"info","description":"Model downloading","downloading":true,"status":"not_installed"}`, 0)
		res, err := idx.PrecheckDownload(t.Context(), cli, url, "", plat)
		require.NoError(t, err)
		assert.False(t, res.Installed)
	})

	t.Run("bigger than the free space: ErrInsufficientStorage", func(t *testing.T) {
		idx, cli, _ := newIndex(t, `{"event":"stat","size_bytes":1152921504606846976}`, notInstalled, 1) // 1 EiB
		_, err := idx.PrecheckDownload(t.Context(), cli, url, "", plat)
		require.ErrorIs(t, err, ErrInsufficientStorage)
	})

	t.Run("a handler without info: size unknown, no error", func(t *testing.T) {
		idx, cli, _ := newIndex(t, "", notInstalled, 1)
		hf, ok := idx.Handlers.GetHandlerByID(hfHandlerID)
		require.True(t, ok)
		hf.ID, hf.Actions.Info = "no-info-handler", nil
		idx.Handlers.handlers[hf.ID] = hf
		model := userHFModel(url, "", plat)
		model.Deployment.Handler = hf.ID

		res, err := idx.precheck(t.Context(), cli, model, url, true, plat)
		require.NoError(t, err)
		assert.Zero(t, res.SizeBytes)
	})
}
