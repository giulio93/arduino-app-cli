// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package orchestrator

// This file wires the `deployment` block introduced by
// arduino/app-bricks-py#227 to the three in-process Go clients added in
// PR #1 of this repo (internal/api/{huggingface,aihub,edgeimpulse}).
//
// The flow: a model in models-list.yaml declares
//
//	deployment:
//	  handler: ai-hub-handler | hf-handler | ei-handler
//	  platforms:
//	    - <board>:
//	        variables: { ... handler-specific keys ... }
//
// InstallModelByID picks the right handler from `handler`, resolves the
// platform-specific `variables`, and invokes the matching client to fetch
// the model.
//
// None of the three download paths require user-supplied credentials:
//   * AI Hub assets are on a public S3 bucket
//   * EI uses the parameterized /deployment/download endpoint (no x-api-key)
//   * HF public repos work without a token (the deployment block may carry
//     hf_token for gated repos)

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-app-cli/internal/api/aihub"
	"github.com/arduino/arduino-app-cli/internal/api/edgeimpulse"
	"github.com/arduino/arduino-app-cli/internal/api/huggingface"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex/custommodel"
)

// Handler IDs as they appear in models-handlers.yaml from PR #227.
const (
	HandlerAIHub = "ai-hub-handler"
	HandlerEI    = "ei-handler"
	HandlerHF    = "hf-handler"

	// defaultHFAPIURL is the public Hugging Face Hub API base URL.
	defaultHFAPIURL = "https://huggingface.co/api/models"
	// hfResolveBase is used as the file-resolution root by the HF client.
	hfDownloadBase = "https://huggingface.co"
)

var (
	ErrNoDeployment           = errors.New("model has no deployment block")
	ErrNoVariablesForPlatform = errors.New("no deployment variables for the requested platform")
	ErrUnknownHandler         = errors.New("unknown deployment handler")
	ErrMissingDeploymentVar   = errors.New("missing required deployment variable")
)

// InstallClients bundles the optional dependencies of the install handlers so
// tests can inject httptest-backed clients without monkey-patching globals.
// All fields are optional; nil/empty values trigger production defaults.
type InstallClients struct {
	// AIHub, when non-nil, is used as-is. Otherwise a default client pointing
	// at the public S3 bucket is constructed.
	AIHub *aihub.AIHubClient

	// HF, when non-nil, is used as-is. Otherwise a default client pointing at
	// https://huggingface.co/api/models is constructed.
	HF *huggingface.HFClient

	// EIBaseURL overrides the EI Studio v1 API base URL (defaults to the
	// public one). EIHTTPClient overrides the HTTP client used for that
	// request only — typically used in tests with httptest.Server.Client().
	EIBaseURL     *url.URL
	EIHTTPClient  *http.Client
}

// InstallModelByID looks up the model in modelsIndex and dispatches to the
// handler named in its deployment block. platformName selects which platforms[]
// entry's variables are used (e.g. "ventunoq").
func InstallModelByID(
	ctx context.Context,
	modelsIndex *modelsindex.ModelsIndex,
	modelsDir *paths.Path,
	modelID, platformName string,
	clients InstallClients,
) (AIModelItem, error) {
	model, found := modelsIndex.GetModelByID(modelID)
	if !found {
		return AIModelItem{}, fmt.Errorf("%q: %w", modelID, ErrNotFound)
	}
	if model.Deployment == nil {
		return AIModelItem{}, fmt.Errorf("%q: %w", modelID, ErrNoDeployment)
	}
	vars := model.VariablesFor(platformName)
	if vars == nil {
		return AIModelItem{}, fmt.Errorf("%q on %q: %w", modelID, platformName, ErrNoVariablesForPlatform)
	}

	switch model.Deployment.Handler {
	case HandlerAIHub:
		return installAIHub(ctx, model, vars, modelsDir, clients.AIHub)
	case HandlerEI:
		return installEI(ctx, model, vars, modelsDir, clients.EIBaseURL, clients.EIHTTPClient)
	case HandlerHF:
		return installHF(ctx, model, vars, modelsDir, clients.HF)
	default:
		return AIModelItem{}, fmt.Errorf("%w: %q", ErrUnknownHandler, model.Deployment.Handler)
	}
}

func mustVar(vars map[string]string, key string) (string, error) {
	v, ok := vars[key]
	if !ok || v == "" {
		return "", fmt.Errorf("%w: %s", ErrMissingDeploymentVar, key)
	}
	return v, nil
}

func optionalVar(vars map[string]string, key string) string {
	return vars[key]
}

// ---------------------------------------------------------------------------
// AI Hub
// ---------------------------------------------------------------------------

func installAIHub(
	ctx context.Context,
	model *modelsindex.AIModel,
	vars map[string]string,
	modelsDir *paths.Path,
	client *aihub.AIHubClient,
) (AIModelItem, error) {
	name, err := mustVar(vars, "model_name")
	if err != nil {
		return AIModelItem{}, err
	}
	runtime, err := mustVar(vars, "model_type")
	if err != nil {
		return AIModelItem{}, err
	}
	quant, err := mustVar(vars, "quantization")
	if err != nil {
		return AIModelItem{}, err
	}
	chipset, err := mustVar(vars, "chipset")
	if err != nil {
		return AIModelItem{}, err
	}
	version, err := mustVar(vars, "version")
	if err != nil {
		return AIModelItem{}, err
	}
	modelDir, err := mustVar(vars, "model_directory")
	if err != nil {
		return AIModelItem{}, err
	}

	// models_repository is optional: when absent we fall back to the orchestrator's
	// configured custom-models directory under a "genai" subdir, mirroring the
	// Python downloader's default.
	dest := paths.New(optionalVar(vars, "models_repository"))
	if dest.String() == "" {
		dest = modelsDir.Join("genai")
	}

	if client == nil {
		defaultURL, perr := url.Parse(aihub.DefaultAPIURL)
		if perr != nil {
			return AIModelItem{}, fmt.Errorf("parse default AI Hub URL: %w", perr)
		}
		client, err = aihub.NewAIHubClient(*defaultURL)
		if err != nil {
			return AIModelItem{}, fmt.Errorf("build AI Hub client: %w", err)
		}
	}

	body, err := client.DownloadModel(ctx, aihub.ModelSpec{
		Name:         name,
		Runtime:      runtime,
		Quantization: quant,
		Chipset:      chipset,
		Version:      version,
	})
	if err != nil {
		return AIModelItem{}, fmt.Errorf("download AI Hub asset for %q: %w", model.ID, err)
	}

	written, err := custommodel.ExtractZipStream(body, dest)
	if err != nil {
		return AIModelItem{}, fmt.Errorf("extract AI Hub asset for %q: %w", model.ID, err)
	}
	slog.Info("AI Hub model installed", "id", model.ID, "files", len(written), "dest", dest)

	// The ZIP wraps everything under model_directory — that's the canonical
	// install location we record on the AIModelItem.
	installDir := dest.Join(modelDir)

	return AIModelItem{
		ID:                model.ID,
		Name:              model.Name,
		ModuleDescription: model.ModuleDescription,
		Runner:            model.Runner,
		Metadata:          mergeMetadata(model.Metadata, map[string]string{"install_dir": installDir.String()}),
	}, nil
}

// ---------------------------------------------------------------------------
// Edge Impulse (parameterized, unauthenticated)
// ---------------------------------------------------------------------------

func installEI(
	ctx context.Context,
	model *modelsindex.AIModel,
	vars map[string]string,
	modelsDir *paths.Path,
	baseURL *url.URL,
	httpClient *http.Client,
) (AIModelItem, error) {
	projectID, err := intVar(vars, "ei_project_id")
	if err != nil {
		return AIModelItem{}, err
	}
	impulseID, err := intVar(vars, "ei_impulse_id")
	if err != nil {
		return AIModelItem{}, err
	}
	modelName, err := mustVar(vars, "model_name")
	if err != nil {
		return AIModelItem{}, err
	}
	quant, err := mustVar(vars, "quantization")
	if err != nil {
		return AIModelItem{}, err
	}
	target, err := mustVar(vars, "target")
	if err != nil {
		return AIModelItem{}, err
	}

	dest := paths.New(optionalVar(vars, "models_repository"))
	if dest.String() == "" {
		dest = modelsDir.Join("ei")
	}
	if err := dest.MkdirAll(); err != nil {
		return AIModelItem{}, fmt.Errorf("create destination dir: %w", err)
	}

	body, err := edgeimpulse.DownloadDeployment(ctx, baseURL, httpClient, projectID, impulseID, target, quant)
	if err != nil {
		return AIModelItem{}, fmt.Errorf("download EI deployment for %q: %w", model.ID, err)
	}

	outPath := dest.Join(modelName)
	if err := writeReadCloserToFile(body, outPath, 0o755); err != nil {
		return AIModelItem{}, fmt.Errorf("write EI model file: %w", err)
	}

	slog.Info("EI model installed", "id", model.ID, "path", outPath)

	return AIModelItem{
		ID:                model.ID,
		Name:              model.Name,
		ModuleDescription: model.ModuleDescription,
		Runner:            model.Runner,
		Metadata:          mergeMetadata(model.Metadata, map[string]string{"install_path": outPath.String()}),
	}, nil
}

func intVar(vars map[string]string, key string) (int, error) {
	s, err := mustVar(vars, key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s is not an int: %w", key, err)
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Hugging Face
// ---------------------------------------------------------------------------

func installHF(
	ctx context.Context,
	model *modelsindex.AIModel,
	vars map[string]string,
	modelsDir *paths.Path,
	client *huggingface.HFClient,
) (AIModelItem, error) {
	repoID, patterns, err := resolveHFTarget(vars)
	if err != nil {
		return AIModelItem{}, err
	}

	dest := paths.New(optionalVar(vars, "models_repository"))
	if dest.String() == "" {
		dest = modelsDir.Join("hf")
	}

	if client == nil {
		baseURL, perr := url.Parse(defaultHFAPIURL)
		if perr != nil {
			return AIModelItem{}, fmt.Errorf("parse default HF URL: %w", perr)
		}
		client, err = huggingface.NewHFClient(optionalVar(vars, "hf_token"), *baseURL)
		if err != nil {
			return AIModelItem{}, fmt.Errorf("build HF client: %w", err)
		}
	}

	matched, err := client.MatchFiles(ctx, repoID, patterns)
	if err != nil {
		return AIModelItem{}, fmt.Errorf("list HF files for %q: %w", repoID, err)
	}
	if len(matched) == 0 {
		return AIModelItem{}, fmt.Errorf("no files in %q match patterns %v", repoID, patterns)
	}

	repoDest := dest.Join(repoID)
	if err := repoDest.MkdirAll(); err != nil {
		return AIModelItem{}, fmt.Errorf("create HF dest: %w", err)
	}

	for _, filename := range matched {
		rc, err := client.DownloadFile(ctx, repoID, filename)
		if err != nil {
			return AIModelItem{}, fmt.Errorf("download %s/%s: %w", repoID, filename, err)
		}
		if err := writeReadCloserToFile(rc, repoDest.Join(filename), 0o644); err != nil {
			return AIModelItem{}, fmt.Errorf("write %s/%s: %w", repoID, filename, err)
		}
	}

	slog.Info("HF model installed", "id", model.ID, "files", len(matched), "dest", repoDest)

	return AIModelItem{
		ID:                model.ID,
		Name:              model.Name,
		ModuleDescription: model.ModuleDescription,
		Runner:            model.Runner,
		Metadata: mergeMetadata(model.Metadata, map[string]string{
			"install_dir": repoDest.String(),
			"hf_repo_id":  repoID,
		}),
	}, nil
}

// resolveHFTarget parses the HF-specific deployment variables and returns the
// repo ID + the fnmatch-style patterns to download. Supports the two input
// shapes used by PR #227's hf_downloader.py:
//
//   - model_key="<type>:<repo_id>:<quantization>[:<mmproj_quant>]"
//   - model_repo_id="…" + model_name="…" + optional model_mmproj_name="…"
func resolveHFTarget(vars map[string]string) (repoID string, patterns []string, err error) {
	if key := vars["model_key"]; key != "" {
		parts := strings.SplitN(key, ":", 4)
		if len(parts) < 3 || parts[1] == "" || parts[2] == "" {
			return "", nil, fmt.Errorf("invalid model_key %q: expected <type>:<repo_id>:<quantization>[:<mmproj_quant>]", key)
		}
		repoID = parts[1]
		patterns = []string{fmt.Sprintf("*%s*.gguf", parts[2])}
		if len(parts) == 4 && parts[3] != "" {
			patterns = append(patterns, fmt.Sprintf("*mmproj*%s*.gguf", parts[3]))
		}
		return repoID, patterns, nil
	}

	repoID = vars["model_repo_id"]
	name := vars["model_name"]
	if repoID == "" || name == "" {
		return "", nil, fmt.Errorf("%w: need model_key or (model_repo_id + model_name)", ErrMissingDeploymentVar)
	}
	patterns = []string{toGlobPattern(name)}
	if mm := vars["model_mmproj_name"]; mm != "" {
		patterns = append(patterns, toGlobPattern(mm))
	}
	return repoID, patterns, nil
}

func toGlobPattern(name string) string {
	if strings.ContainsAny(name, "*?") {
		return name
	}
	if strings.HasSuffix(name, ".gguf") {
		return name
	}
	return "*" + name + "*"
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

func writeReadCloserToFile(rc io.ReadCloser, dest *paths.Path, mode os.FileMode) error {
	defer rc.Close()
	if err := dest.Parent().MkdirAll(); err != nil {
		return err
	}
	out, err := dest.Create()
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, rc); err != nil {
		_ = dest.Remove()
		return err
	}
	if mode != 0 {
		if err := dest.Chmod(mode); err != nil {
			slog.Warn("chmod failed", "path", dest, "err", err)
		}
	}
	return nil
}

func mergeMetadata(base, extra map[string]string) map[string]string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
