// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package aihub

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

func newTestClient(t *testing.T, handler http.Handler) *AIHubClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewAIHubClient(*u)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAssetURLs_ChipsetUnderscored(t *testing.T) {
	u, _ := url.Parse("https://example.test/qai-hub-models/models")
	c, _ := NewAIHubClient(*u)
	urls := c.AssetURLs(ModelSpec{
		Name: "melotts_en", Runtime: "voice_ai", Quantization: "mixed_with_float",
		Chipset: "qualcomm-qcs8275", Version: "0.51.0",
	})
	if len(urls) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(urls))
	}
	want := "qualcomm_qcs8275"
	if !strings.Contains(urls[0], want) {
		t.Errorf("first URL missing underscored chipset %q: %s", want, urls[0])
	}
	if strings.Contains(urls[1], want) {
		t.Errorf("fallback URL should omit chipset: %s", urls[1])
	}
	for _, u := range urls {
		if !strings.HasSuffix(u, ".zip") {
			t.Errorf("expected .zip URL, got %s", u)
		}
	}
}

func TestResolveAssetURL_ChipsetSpecific(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD, got %s", r.Method)
		}
		if strings.Contains(r.URL.Path, "qualcomm_qcs8275") {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	got, err := c.ResolveAssetURL(context.Background(), ModelSpec{
		Name: "m", Runtime: "voice_ai", Quantization: "w8a16",
		Chipset: "qualcomm-qcs8275", Version: "0.51.0",
	})
	if err != nil {
		t.Fatalf("ResolveAssetURL: %v", err)
	}
	if !strings.Contains(got, "qualcomm_qcs8275") {
		t.Errorf("expected chipset-specific URL, got %s", got)
	}
}

func TestResolveAssetURL_FallbackGeneric(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "qualcomm_qcs8275") {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))

	got, err := c.ResolveAssetURL(context.Background(), ModelSpec{
		Name: "m", Runtime: "voice_ai", Quantization: "w8a16",
		Chipset: "qualcomm-qcs8275", Version: "0.51.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "qualcomm_qcs8275") {
		t.Errorf("expected generic URL, got chipset-specific: %s", got)
	}
}

func TestResolveAssetURL_NotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := c.ResolveAssetURL(context.Background(), ModelSpec{
		Name: "x", Runtime: "y", Quantization: "z", Chipset: "c", Version: "1.0",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestDownloadModel(t *testing.T) {
	want := "model-zip-bytes"
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			if strings.Contains(r.URL.Path, "qualcomm_qcs8275") {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case http.MethodGet:
			w.Write([]byte(want))
		}
	}))

	rc, err := c.DownloadModel(context.Background(), ModelSpec{
		Name: "m", Runtime: "voice_ai", Quantization: "w8a16",
		Chipset: "qualcomm-qcs8275", Version: "0.51.0",
	})
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
