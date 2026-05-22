// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package aihub provides a thin client for downloading models published on
// Qualcomm AI Hub. Models are served from a public S3 bucket so the client
// does not require authentication; the qai-hub-models Python CLI is only
// used to assemble the asset URL, which this client replicates in Go.
package aihub

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// AIHubClient resolves and downloads Qualcomm AI Hub model assets.
type AIHubClient struct {
	ApiUrl     url.URL
	HttpClient *http.Client
}

var (
	ErrInternalServerErr = fmt.Errorf("service unavailable")
	ErrNotFound          = fmt.Errorf("model asset not found")
)

// DefaultAPIURL points at the public AI Hub asset bucket.
const DefaultAPIURL = "https://qaihub-public-assets.s3.us-west-2.amazonaws.com/qai-hub-models/models"

// ModelSpec uniquely identifies a model asset on AI Hub.
type ModelSpec struct {
	// Name is the model identifier, e.g. "melotts_en".
	Name string
	// Runtime is the runtime/engine, e.g. "voice_ai", "genie", "qnn_dlc".
	Runtime string
	// Quantization is the precision, e.g. "mixed_with_float", "w4a16".
	Quantization string
	// Chipset uses the hyphenated form, e.g. "qualcomm-qcs8275". The client
	// converts hyphens to underscores when building the URL.
	Chipset string
	// Version is the release version, e.g. "0.51.0".
	Version string
}

// NewAIHubClient creates a new client. apiURL should normally be parsed
// from DefaultAPIURL; pass a different URL only for testing.
func NewAIHubClient(apiURL url.URL) (*AIHubClient, error) {
	return &AIHubClient{
		ApiUrl:     apiURL,
		HttpClient: &http.Client{},
	}, nil
}

// AssetURLs returns the candidate URLs for spec in resolution order:
// chipset-specific first, then a chipset-less fallback.
func (c *AIHubClient) AssetURLs(spec ModelSpec) []string {
	chipsetUnderscored := strings.ReplaceAll(spec.Chipset, "-", "_")
	base := fmt.Sprintf("%s/%s/releases/v%s", strings.TrimRight(c.ApiUrl.String(), "/"), spec.Name, spec.Version)
	return []string{
		fmt.Sprintf("%s/%s-%s-%s-%s.zip", base, spec.Name, spec.Runtime, spec.Quantization, chipsetUnderscored),
		fmt.Sprintf("%s/%s-%s-%s.zip", base, spec.Name, spec.Runtime, spec.Quantization),
	}
}

// ResolveAssetURL returns the first candidate URL that responds 200 to a HEAD
// request, or ErrNotFound if none do.
func (c *AIHubClient) ResolveAssetURL(ctx context.Context, spec ModelSpec) (string, error) {
	for _, u := range c.AssetURLs(spec) {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
		if err != nil {
			return "", fmt.Errorf("build AI Hub HEAD request: %w", err)
		}
		resp, err := c.HttpClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return u, nil
		}
	}
	return "", fmt.Errorf("%w: model=%q runtime=%q quantization=%q chipset=%q version=%q",
		ErrNotFound, spec.Name, spec.Runtime, spec.Quantization, spec.Chipset, spec.Version)
}

// DownloadAsset streams the model ZIP from the given URL. The caller is
// responsible for closing the returned ReadCloser. URL must be obtained from
// ResolveAssetURL — passing an arbitrary URL is not validated here.
func (c *AIHubClient) DownloadAsset(ctx context.Context, assetURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build AI Hub download request: %w", err)
	}
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform AI Hub download request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %d %s — %s", ErrInternalServerErr, resp.StatusCode, resp.Status, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

// DownloadModel is a convenience wrapper that resolves the URL and starts the
// download in one call.
func (c *AIHubClient) DownloadModel(ctx context.Context, spec ModelSpec) (io.ReadCloser, error) {
	assetURL, err := c.ResolveAssetURL(ctx, spec)
	if err != nil {
		return nil, err
	}
	return c.DownloadAsset(ctx, assetURL)
}
