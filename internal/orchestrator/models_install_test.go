// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package orchestrator

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-app-cli/internal/api/aihub"
	"github.com/arduino/arduino-app-cli/internal/api/huggingface"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
)

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

func newIndex(models ...modelsindex.AIModel) *modelsindex.ModelsIndex {
	return &modelsindex.ModelsIndex{InternalModels: models}
}

func mustParseURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	require.NoError(t, err)
	return u
}

func buildTestZip(t *testing.T, entries map[string]string) []byte {
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
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// AI Hub install
// ---------------------------------------------------------------------------

func TestInstallModelByID_AIHub(t *testing.T) {
	// Asset name pattern: {model_name}-{model_type}-{quantization}-{chipset}.zip
	// We make the ZIP wrap everything under a top-level dir that matches
	// model_directory, the way real AI Hub assets do.
	const modelDir = "qwen3_4b_instruct_2507-genie-w4a16-qualcomm_qcs8275"
	zipBytes := buildTestZip(t, map[string]string{
		modelDir + "/":                "",
		modelDir + "/model.bin":       "weights-payload",
		modelDir + "/config.json":     `{"runtime":"genie"}`,
		modelDir + "/tokenizer.model": "tok",
	})

	var (
		headCalls = 0
		getCalls  = 0
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Both HEAD probe and GET hit the same URL; serve the ZIP for both.
		if !strings.HasSuffix(r.URL.Path, ".zip") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodHead:
			headCalls++
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			getCalls++
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes)
		}
	}))
	defer srv.Close()

	client, err := aihub.NewAIHubClient(*mustParseURL(t, srv.URL))
	require.NoError(t, err)

	tmpRepo := t.TempDir()

	model := modelsindex.AIModel{
		ID:                "genie:qwen3_4b_instruct_2507",
		Name:              "Qwen 3-4B Instruct",
		ModuleDescription: "test",
		Deployment: &modelsindex.Deployment{
			Handler: HandlerAIHub,
			Platforms: []modelsindex.DeploymentPlatform{
				{Name: "ventunoq", Variables: map[string]string{
					"model_type":        "genie",
					"model_name":        "qwen3_4b_instruct_2507",
					"models_repository": tmpRepo,
					"model_directory":   modelDir,
					"quantization":      "w4a16",
					"chipset":           "qualcomm-qcs8275",
					"version":           "0.51.0",
				}},
			},
		},
	}

	item, err := InstallModelByID(
		context.Background(),
		newIndex(model),
		paths.New(t.TempDir()),
		model.ID, "ventunoq",
		InstallClients{AIHub: client},
	)
	require.NoError(t, err)

	// Resulting AIModelItem points at the expected install dir.
	assert.Equal(t, model.ID, item.ID)
	assert.Equal(t, paths.New(tmpRepo).Join(modelDir).String(), item.Metadata["install_dir"])

	// Files were extracted in the expected layout.
	assert.True(t, paths.New(tmpRepo).Join(modelDir, "model.bin").Exist(),
		"extracted model file should exist")
	assert.True(t, paths.New(tmpRepo).Join(modelDir, "config.json").Exist())

	// HEAD probed first, then a GET pulled the ZIP.
	assert.Equal(t, 1, headCalls, "exactly one HEAD probe")
	assert.Equal(t, 1, getCalls, "exactly one GET download")
}

func TestInstallModelByID_AIHub_FailsWithoutVariables(t *testing.T) {
	model := modelsindex.AIModel{
		ID: "test",
		Deployment: &modelsindex.Deployment{
			Handler: HandlerAIHub,
			Platforms: []modelsindex.DeploymentPlatform{
				{Name: "ventunoq", Variables: map[string]string{
					"model_type": "genie",
					// missing required keys
				}},
			},
		},
	}

	_, err := InstallModelByID(
		context.Background(),
		newIndex(model),
		paths.New(t.TempDir()),
		model.ID, "ventunoq",
		InstallClients{},
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingDeploymentVar))
}

// ---------------------------------------------------------------------------
// Edge Impulse install (parameterized, unauth)
// ---------------------------------------------------------------------------

func TestInstallModelByID_EI(t *testing.T) {
	const wantBody = "edge-impulse-model-bytes"

	var capturedAuth string
	var capturedQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("x-api-key")
		capturedQuery = r.URL.Query()
		_, _ = io.WriteString(w, wantBody)
	}))
	defer srv.Close()

	tmpRepo := t.TempDir()

	model := modelsindex.AIModel{
		ID:                "ei-yolox",
		Name:              "YoloX",
		ModuleDescription: "test",
		Deployment: &modelsindex.Deployment{
			Handler: HandlerEI,
			Platforms: []modelsindex.DeploymentPlatform{
				{Name: "ventunoq", Variables: map[string]string{
					"ei_project_id":     "948887",
					"ei_impulse_id":     "11",
					"model_name":        "yolox.eim",
					"quantization":      "int8",
					"target":            "runner-linux-aarch64",
					"models_repository": tmpRepo,
				}},
			},
		},
	}

	item, err := InstallModelByID(
		context.Background(),
		newIndex(model),
		paths.New(t.TempDir()),
		model.ID, "ventunoq",
		InstallClients{
			EIBaseURL:    mustParseURL(t, srv.URL+"/v1/api"),
			EIHTTPClient: srv.Client(),
		},
	)
	require.NoError(t, err)

	// The .eim file landed under models_repository/{model_name}.
	outPath := paths.New(tmpRepo).Join("yolox.eim")
	assert.Equal(t, outPath.String(), item.Metadata["install_path"])
	got, err := outPath.ReadFile()
	require.NoError(t, err)
	assert.Equal(t, wantBody, string(got))

	// Critical: no x-api-key header was sent — this endpoint is unauth.
	assert.Empty(t, capturedAuth, "EI download must be unauthenticated")

	// Query parameters match PR #227's expected shape.
	assert.Equal(t, "runner-linux-aarch64", capturedQuery.Get("type"))
	assert.Equal(t, "int8", capturedQuery.Get("modelType"))
	assert.Equal(t, "11", capturedQuery.Get("impulseId"))
}

// ---------------------------------------------------------------------------
// Hugging Face install
// ---------------------------------------------------------------------------

func TestInstallModelByID_HF(t *testing.T) {
	const repoID = "unsloth/gemma-4-E4B-it-GGUF"

	// HF API hosts the repo manifest at /api/models/{repoID} and the file
	// content at /{repoID}/resolve/main/{filename}. We mount one mux that
	// serves both endpoints.
	mux := http.NewServeMux()
	manifestPath := "/api/models/" + repoID
	mux.HandleFunc(manifestPath, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"siblings": []map[string]string{
				{"rfilename": "config.json"},
				{"rfilename": "gemma-4-E4B-it-Q4_0.gguf"},
				{"rfilename": "gemma-4-E4B-it-Q8_0.gguf"},
				{"rfilename": "mmproj-BF16.gguf"},
			},
		})
	})
	mux.HandleFunc("/"+repoID+"/resolve/main/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "weights:"+r.URL.Path)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// HFClient's ApiUrl is the HF host root; it appends /api/models/{repoID}
	// for listing and /{repoID}/resolve/main/{file} for downloads itself.
	client, err := huggingface.NewHFClient("", *mustParseURL(t, srv.URL))
	require.NoError(t, err)

	tmpRepo := t.TempDir()

	model := modelsindex.AIModel{
		ID:                "hf-gemma",
		Name:              "Gemma 4",
		ModuleDescription: "test",
		Deployment: &modelsindex.Deployment{
			Handler: HandlerHF,
			Platforms: []modelsindex.DeploymentPlatform{
				{Name: "ventunoq", Variables: map[string]string{
					"model_key":         "llamacpp:" + repoID + ":Q4_0",
					"models_repository": tmpRepo,
				}},
			},
		},
	}

	item, err := InstallModelByID(
		context.Background(),
		newIndex(model),
		paths.New(t.TempDir()),
		model.ID, "ventunoq",
		InstallClients{HF: client},
	)
	require.NoError(t, err)

	assert.Equal(t, repoID, item.Metadata["hf_repo_id"])
	assert.Equal(t, paths.New(tmpRepo).Join(repoID).String(), item.Metadata["install_dir"])

	// Only the matching file was downloaded (mmproj excluded because no
	// :<mmproj_quant> suffix in model_key, Q8_0 excluded by the Q4_0 pattern).
	downloaded := paths.New(tmpRepo).Join(repoID, "gemma-4-E4B-it-Q4_0.gguf")
	assert.True(t, downloaded.Exist(), "Q4_0 file should be downloaded")
	assert.False(t, paths.New(tmpRepo).Join(repoID, "gemma-4-E4B-it-Q8_0.gguf").Exist(),
		"Q8_0 should be filtered out")
	assert.False(t, paths.New(tmpRepo).Join(repoID, "mmproj-BF16.gguf").Exist(),
		"mmproj should be filtered out when not requested")
}

func TestInstallModelByID_HF_WithMmproj(t *testing.T) {
	const repoID = "unsloth/gemma-4-E4B-it-GGUF"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/"+repoID, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"siblings": []map[string]string{
				{"rfilename": "gemma-Q4_0.gguf"},
				{"rfilename": "mmproj-BF16.gguf"},
			},
		})
	})
	mux.HandleFunc("/"+repoID+"/resolve/main/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.Path)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := huggingface.NewHFClient("", *mustParseURL(t, srv.URL))
	require.NoError(t, err)

	model := modelsindex.AIModel{
		ID: "hf-gemma-vlm",
		Deployment: &modelsindex.Deployment{
			Handler: HandlerHF,
			Platforms: []modelsindex.DeploymentPlatform{
				{Name: "ventunoq", Variables: map[string]string{
					// Trailing :BF16 → adds the mmproj pattern.
					"model_key":         "llamacpp:" + repoID + ":Q4_0:BF16",
					"models_repository": t.TempDir(),
				}},
			},
		},
	}

	_, err = InstallModelByID(
		context.Background(),
		newIndex(model),
		paths.New(t.TempDir()),
		model.ID, "ventunoq",
		InstallClients{HF: client},
	)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// dispatch & error paths
// ---------------------------------------------------------------------------

func TestInstallModelByID_UnknownHandler(t *testing.T) {
	model := modelsindex.AIModel{
		ID: "weird",
		Deployment: &modelsindex.Deployment{
			Handler: "nope-handler",
			Platforms: []modelsindex.DeploymentPlatform{
				{Name: "ventunoq", Variables: map[string]string{"x": "y"}},
			},
		},
	}
	_, err := InstallModelByID(context.Background(), newIndex(model),
		paths.New(t.TempDir()), model.ID, "ventunoq", InstallClients{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnknownHandler))
}

func TestInstallModelByID_MissingModel(t *testing.T) {
	_, err := InstallModelByID(context.Background(), newIndex(),
		paths.New(t.TempDir()), "does-not-exist", "ventunoq", InstallClients{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestInstallModelByID_NoDeploymentBlock(t *testing.T) {
	model := modelsindex.AIModel{ID: "legacy", Runner: "brick"}
	_, err := InstallModelByID(context.Background(), newIndex(model),
		paths.New(t.TempDir()), model.ID, "ventunoq", InstallClients{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoDeployment))
}

func TestInstallModelByID_NoVariablesForPlatform(t *testing.T) {
	model := modelsindex.AIModel{
		ID: "test",
		Deployment: &modelsindex.Deployment{
			Handler: HandlerAIHub,
			Platforms: []modelsindex.DeploymentPlatform{
				{Name: "ventunoq", Variables: map[string]string{}},
			},
		},
	}
	_, err := InstallModelByID(context.Background(), newIndex(model),
		paths.New(t.TempDir()), model.ID, "some-other-board", InstallClients{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoVariablesForPlatform))
}

// ---------------------------------------------------------------------------
// HF target resolution (unit)
// ---------------------------------------------------------------------------

func TestResolveHFTarget(t *testing.T) {
	t.Run("model_key without mmproj", func(t *testing.T) {
		repo, patterns, err := resolveHFTarget(map[string]string{
			"model_key": "llamacpp:org/repo:Q4_0",
		})
		require.NoError(t, err)
		assert.Equal(t, "org/repo", repo)
		assert.Equal(t, []string{"*Q4_0*.gguf"}, patterns)
	})

	t.Run("model_key with mmproj", func(t *testing.T) {
		repo, patterns, err := resolveHFTarget(map[string]string{
			"model_key": "llamacpp:org/repo:Q4_0:BF16",
		})
		require.NoError(t, err)
		assert.Equal(t, "org/repo", repo)
		assert.Equal(t, []string{"*Q4_0*.gguf", "*mmproj*BF16*.gguf"}, patterns)
	})

	t.Run("explicit repo + name", func(t *testing.T) {
		repo, patterns, err := resolveHFTarget(map[string]string{
			"model_repo_id": "org/repo",
			"model_name":    "model.gguf",
		})
		require.NoError(t, err)
		assert.Equal(t, "org/repo", repo)
		assert.Equal(t, []string{"model.gguf"}, patterns)
	})

	t.Run("explicit non-gguf name gets wildcarded", func(t *testing.T) {
		_, patterns, err := resolveHFTarget(map[string]string{
			"model_repo_id": "org/repo",
			"model_name":    "Q4_0",
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"*Q4_0*"}, patterns)
	})

	t.Run("invalid model_key", func(t *testing.T) {
		_, _, err := resolveHFTarget(map[string]string{
			"model_key": "llamacpp:org/repo", // missing quantization
		})
		require.Error(t, err)
	})

	t.Run("missing both shapes", func(t *testing.T) {
		_, _, err := resolveHFTarget(map[string]string{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrMissingDeploymentVar))
	})
}
