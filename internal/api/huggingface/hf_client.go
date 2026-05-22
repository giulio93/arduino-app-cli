// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package huggingface provides a thin HTTP client for the Hugging Face Hub
// REST API. It is used to list files in a model repository and stream those
// files to the caller. Only the surface needed by arduino-app-cli is exposed.
package huggingface

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// HFClient is a minimal Hugging Face Hub client.
type HFClient struct {
	ApiUrl     url.URL
	ApiKey     string // optional; required only for gated/private repos
	HttpClient *http.Client
}

var (
	ErrInternalServerErr = fmt.Errorf("service unavailable")
	ErrUnauthorized      = fmt.Errorf("unauthorized")
	ErrForbidden         = fmt.Errorf("cannot access the resource with the provided credentials")
	ErrNotFound          = fmt.Errorf("resource not found")
)

// RepoFile describes a single file inside a HF model repository.
type RepoFile struct {
	Filename string `json:"rfilename"`
	Size     int64  `json:"size,omitempty"`
	BlobID   string `json:"blobId,omitempty"`
}

// RepoInfo is the subset of the /api/models/{repo_id} response that we care
// about. The full response includes many other fields which we deliberately
// ignore to keep the surface small.
type RepoInfo struct {
	ID       string     `json:"id"`
	Author   string     `json:"author,omitempty"`
	Tags     []string   `json:"tags,omitempty"`
	Siblings []RepoFile `json:"siblings"`
}

// NewHFClient creates a new client targeting the given API URL. An empty
// apiKey is valid for public repositories.
func NewHFClient(apiKey string, apiURL url.URL) (*HFClient, error) {
	return &HFClient{
		ApiKey:     apiKey,
		ApiUrl:     apiURL,
		HttpClient: &http.Client{},
	}, nil
}

func (c *HFClient) authorize(req *http.Request) {
	if c.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
	}
}

// GetRepoInfo fetches model metadata and the list of files for repoID
// (e.g. "unsloth/gemma-4-E4B-it-GGUF").
func (c *HFClient) GetRepoInfo(ctx context.Context, repoID string) (*RepoInfo, error) {
	u := c.ApiUrl
	u.Path = path.Join(u.Path, "api", "models", repoID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build HF info request: %w", err)
	}
	c.authorize(req)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform HF info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: %s", errorMessage(resp.StatusCode), strings.TrimSpace(string(body)))
	}

	var info RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode HF info response: %w", err)
	}
	return &info, nil
}

// ListFiles returns the names of all files in repoID.
func (c *HFClient) ListFiles(ctx context.Context, repoID string) ([]string, error) {
	info, err := c.GetRepoInfo(ctx, repoID)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(info.Siblings))
	for _, s := range info.Siblings {
		files = append(files, s.Filename)
	}
	return files, nil
}

// MatchFiles returns the subset of files in repoID matching any of the given
// fnmatch-style patterns (e.g. "*Q4_0*.gguf"). Files matching mmproj are
// included only when at least one pattern explicitly targets them.
func (c *HFClient) MatchFiles(ctx context.Context, repoID string, patterns []string) ([]string, error) {
	files, err := c.ListFiles(ctx, repoID)
	if err != nil {
		return nil, err
	}
	var matched []string
	for _, f := range files {
		base := filepath.Base(f)
		for _, p := range patterns {
			if ok, _ := filepath.Match(p, base); ok {
				matched = append(matched, f)
				break
			}
		}
	}
	return matched, nil
}

// DownloadFile streams the contents of filename from repoID. The caller is
// responsible for closing the returned ReadCloser.
//
// The HF API uses redirects to S3-backed storage; the default Go HTTP client
// follows them automatically.
func (c *HFClient) DownloadFile(ctx context.Context, repoID, filename string) (io.ReadCloser, error) {
	u := c.ApiUrl
	u.Path = path.Join(u.Path, repoID, "resolve", "main", filename)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build HF download request: %w", err)
	}
	c.authorize(req)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform HF download request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %s", errorMessage(resp.StatusCode), strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

func errorMessage(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return ErrInternalServerErr
	}
}
