// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package huggingface

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func newTestClient(t *testing.T, handler http.Handler) *HFClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewHFClient("test-token", *u)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestGetRepoInfo(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models/org/repo" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(RepoInfo{
			ID:     "org/repo",
			Author: "org",
			Siblings: []RepoFile{
				{Filename: "model-Q4_0.gguf"},
				{Filename: "config.json"},
			},
		})
	}))

	info, err := client.GetRepoInfo(context.Background(), "org/repo")
	if err != nil {
		t.Fatalf("GetRepoInfo: %v", err)
	}
	if info.ID != "org/repo" {
		t.Errorf("ID = %q, want org/repo", info.ID)
	}
	if len(info.Siblings) != 2 {
		t.Errorf("siblings = %d, want 2", len(info.Siblings))
	}
}

func TestGetRepoInfo_Unauthorized(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	}))

	_, err := client.GetRepoInfo(context.Background(), "private/repo")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("got %v, want ErrUnauthorized", err)
	}
}

func TestGetRepoInfo_NotFound(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := client.GetRepoInfo(context.Background(), "missing/repo")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestMatchFiles(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(RepoInfo{
			Siblings: []RepoFile{
				{Filename: "model-Q4_0.gguf"},
				{Filename: "model-Q8_0.gguf"},
				{Filename: "mmproj-BF16.gguf"},
				{Filename: "README.md"},
			},
		})
	}))

	got, err := client.MatchFiles(context.Background(), "org/repo", []string{"*Q4_0*.gguf"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "model-Q4_0.gguf" {
		t.Errorf("MatchFiles: got %v", got)
	}

	got, err = client.MatchFiles(context.Background(), "org/repo", []string{"*Q4_0*.gguf", "*mmproj*"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 matches (Q4_0 + mmproj), got %v", got)
	}
}

func TestDownloadFile(t *testing.T) {
	want := "binary model data"
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/org/repo/resolve/main/model.gguf" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(want))
	}))

	rc, err := client.DownloadFile(context.Background(), "org/repo", "model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDownloadFile_Error(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusForbidden)
	}))

	_, err := client.DownloadFile(context.Background(), "org/repo", "model.gguf")
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("got %v, want ErrForbidden", err)
	}
}

func TestAuth_OmittedWhenNoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Authorization"); h != "" {
			t.Errorf("expected no auth header, got %q", h)
		}
		json.NewEncoder(w).Encode(RepoInfo{ID: "x"})
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c, _ := NewHFClient("", *u)
	if _, err := c.GetRepoInfo(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
}
