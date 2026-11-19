// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appbook

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The CRUD over HTTP, as a caller of the package sees it.
func TestNew(t *testing.T) {
	tmp := t.TempDir()
	assets := filepath.Join(tmp, "data", "assets")
	require.NoError(t, os.MkdirAll(assets, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(assets, "bricks-list.yaml"), []byte("bricks: []\n"), 0o600))
	router, err := New(Config{AppsDir: filepath.Join(tmp, "apps"), DataDir: filepath.Join(tmp, "data")})
	require.NoError(t, err)
	serve := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}

	rec := serve("POST", "/v1/apps", `{"name":"app1"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct{ ID string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = serve("GET", "/v1/apps?filter=apps", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"name":"app1"`)

	rec = serve("PATCH", "/v1/apps/"+created.ID, `{"name":"app2"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var renamed struct{ ID string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &renamed))
	require.DirExists(t, filepath.Join(tmp, "apps", "app2"))

	rec = serve("GET", "/v1/apps/"+renamed.ID, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"name":"app2"`)

	rec = serve("DELETE", "/v1/apps/"+renamed.ID, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoDirExists(t, filepath.Join(tmp, "apps", "app2"))
}

func TestNewRejectsBadConfig(t *testing.T) {
	_, err := New(Config{})
	require.ErrorContains(t, err, "AppsDir is empty")
	_, err = New(Config{AppsDir: t.TempDir()})
	require.ErrorContains(t, err, "DataDir is empty")
}

func TestNewRelativeDirs(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join("data", "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("data", "assets", "bricks-list.yaml"), []byte("bricks: []\n"), 0o600))
	router, err := New(Config{AppsDir: "apps", DataDir: "data"})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"name":"app1"}`)))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct{ ID string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	id, err := base64.RawURLEncoding.DecodeString(created.ID)
	require.NoError(t, err)
	require.Equal(t, "user:app1", string(id))
}

// An app's target is written at create, changed by an edit, and read back.
func TestNewTarget(t *testing.T) {
	tmp := t.TempDir()
	assets := filepath.Join(tmp, "data", "assets")
	require.NoError(t, os.MkdirAll(assets, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(assets, "bricks-list.yaml"), []byte("bricks: []\n"), 0o600))
	router, err := New(Config{AppsDir: filepath.Join(tmp, "apps"), DataDir: filepath.Join(tmp, "data"), Board: "unoq"})
	require.NoError(t, err)
	serve := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}

	rec := serve("POST", "/v1/apps", `{"name":"app1","target":"unoq"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct{ ID string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.FileExists(t, filepath.Join(tmp, "apps", "app1", "app.yaml"))
	yml, err := os.ReadFile(filepath.Join(tmp, "apps", "app1", "app.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(yml), "target: unoq")

	rec = serve("PATCH", "/v1/apps/"+created.ID, `{"target":"ventunoq"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = serve("GET", "/v1/apps/"+created.ID, "")
	require.Contains(t, rec.Body.String(), `"target":"ventunoq"`)
	rec = serve("GET", "/v1/apps?filter=apps", "")
	require.Contains(t, rec.Body.String(), `"target":"ventunoq"`)

	rec = serve("PATCH", "/v1/apps/"+created.ID, `{"target":"esp32"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	rec = serve("POST", "/v1/apps", `{"name":"app2","target":"nope"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestNewRejectsUnknownBoard(t *testing.T) {
	_, err := New(Config{AppsDir: t.TempDir(), DataDir: t.TempDir(), Board: "nope"})
	require.ErrorContains(t, err, "unknown board")
}
