// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package edgeimpulse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

func TestDownloadDeployment_BuildsRequestCorrectly(t *testing.T) {
	const wantBody = "binary-model-bytes"

	var capturedPath, capturedQuery, capturedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		capturedAuth = r.Header.Get("x-api-key")
		_, _ = io.WriteString(w, wantBody)
	}))
	defer srv.Close()

	base := mustParseURL(t, srv.URL+"/v1/api")
	body, err := DownloadDeployment(context.Background(), base, srv.Client(),
		948887, 11, "runner-linux-aarch64", "int8")
	if err != nil {
		t.Fatalf("DownloadDeployment: %v", err)
	}
	defer body.Close()

	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body: got %q want %q", got, wantBody)
	}

	// Verify the request URL exactly matches PR #227's pattern:
	//   GET /v1/api/{projectID}/deployment/download?impulseId=...&modelType=...&type=...
	wantPath := "/v1/api/948887/deployment/download"
	if capturedPath != wantPath {
		t.Errorf("path: got %q want %q", capturedPath, wantPath)
	}

	// Query order is encoded alphabetically by url.Values; assert each key/value.
	q, err := url.ParseQuery(capturedQuery)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if q.Get("type") != "runner-linux-aarch64" {
		t.Errorf("type: got %q", q.Get("type"))
	}
	if q.Get("modelType") != "int8" {
		t.Errorf("modelType: got %q", q.Get("modelType"))
	}
	if q.Get("impulseId") != "11" {
		t.Errorf("impulseId: got %q", q.Get("impulseId"))
	}

	// Crucially: no auth header was sent.
	if capturedAuth != "" {
		t.Errorf("unexpected x-api-key header sent: %q (this endpoint is unauthenticated)", capturedAuth)
	}
}

func TestDownloadDeployment_HandlesBaseURLWithTrailingSlash(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	base := mustParseURL(t, srv.URL+"/v1/api/")
	body, err := DownloadDeployment(context.Background(), base, srv.Client(), 1, 2, "t", "m")
	if err != nil {
		t.Fatal(err)
	}
	body.Close()

	if !strings.HasPrefix(capturedPath, "/v1/api/1/deployment/download") {
		t.Errorf("trailing slash not normalised: %q", capturedPath)
	}
	// And no double slash:
	if strings.Contains(capturedPath, "//") {
		t.Errorf("double slash in path: %q", capturedPath)
	}
}

func TestDownloadDeployment_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	base := mustParseURL(t, srv.URL+"/v1/api")
	_, err := DownloadDeployment(context.Background(), base, srv.Client(), 1, 2, "t", "m")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	// 404 maps via errorMessage() to ErrInternalServerErr (the default branch).
	if !errors.Is(err, ErrInternalServerErr) {
		t.Errorf("expected ErrInternalServerErr, got %v", err)
	}
}

func TestDownloadDeployment_401Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	defer srv.Close()

	base := mustParseURL(t, srv.URL+"/v1/api")
	_, err := DownloadDeployment(context.Background(), base, srv.Client(), 1, 2, "t", "m")
	if err == nil || !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestDownloadDeployment_NilHTTPClient_UsesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	base := mustParseURL(t, srv.URL+"/v1/api")
	body, err := DownloadDeployment(context.Background(), base, nil, 1, 2, "t", "m")
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
}
