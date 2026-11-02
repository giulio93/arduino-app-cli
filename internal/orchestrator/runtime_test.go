// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.bug.st/f"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appid"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
)

func TestNilRuntime(t *testing.T) {
	cfg := setTestOrchestratorConfig(t)
	idProvider := appid.NewAppProvider(cfg, unoQPlatform)
	_, err := CreateApp(CreateAppRequest{Name: "boardless"}, &bricksindex.BricksIndex{}, idProvider, cfg)
	require.NoError(t, err)
	userApp := f.Must(app.Load(cfg.AppsDir().Join("boardless")))

	t.Run("list leaves status empty", func(t *testing.T) {
		res, err := ListApps(t.Context(), nil, ListAppRequest{ShowApps: true}, idProvider, &bricksindex.BricksIndex{}, cfg, unoQPlatform)
		require.NoError(t, err)
		require.Len(t, res.Apps, 1)
		require.Empty(t, res.Apps[0].Status)
	})

	t.Run("details leaves status empty", func(t *testing.T) {
		res, err := AppDetails(t.Context(), nil, userApp, &bricksindex.BricksIndex{}, idProvider, cfg)
		require.NoError(t, err)
		require.Empty(t, res.Status)
	})

	t.Run("rename skips running check", func(t *testing.T) {
		require.NoError(t, EditApp(t.Context(), nil, AppEditRequest{Name: new("boardless-renamed")}, &userApp, cfg))
		userApp = f.Must(app.Load(cfg.AppsDir().Join("boardless-renamed")))
	})

	t.Run("delete skips stop", func(t *testing.T) {
		require.NoError(t, DeleteApp(t.Context(), nil, unoQPlatform, userApp, cfg))
		require.True(t, userApp.FullPath.NotExist())
	})
}
