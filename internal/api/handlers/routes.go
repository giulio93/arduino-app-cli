// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package handlers

import (
	"net/http"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/appid"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appstore"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricks"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/servicesindex"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

// Deps is what the routes that need no board use. Runtime is nil where apps cannot run
// (boardless mode).
type Deps struct {
	Version       string
	Config        config.Configuration
	Platform      platform.Platform
	Runtime       appstore.Runtime
	BricksIndex   *bricksindex.BricksIndex
	ModelsIndex   *modelsindex.ModelsIndex
	ServicesIndex *servicesindex.ServicesIndex
	BrickService  *bricks.Service
	IDProvider    *appid.Provider
	// Models installs models; nil answers needs_board.
	Models ModelInstaller
}

// Register adds the routes that need no board: the board daemon serves them next to its
// own, and pkg/boardless serves them alone.
func Register(mux *http.ServeMux, d Deps) {
	mux.Handle("GET /v1/version", HandlerVersion(d.Version))
	mux.Handle("GET /v1/config", HandleConfig(d.Config))
	mux.Handle("GET /v1/examples", HandleExamples(d.Config, d.BricksIndex, d.IDProvider))
	mux.Handle("GET /v1/bricks", HandleBrickList(d.BrickService))
	mux.Handle("GET /v1/bricks/{brickID}", HandleBrickDetails(d.BrickService, d.IDProvider, d.Config, d.Platform))

	mux.Handle("GET /v1/properties", HandlePropertyKeys(d.Config))
	mux.Handle("GET /v1/properties/{key}", HandlePropertyGet(d.Config))
	mux.Handle("PUT /v1/properties/{key}", HandlePropertyUpsert(d.Config))
	mux.Handle("DELETE /v1/properties/{key}", HandlePropertyDelete(d.Config))

	mux.Handle("GET /v1/models", HandleModelsList(d.ModelsIndex))
	mux.Handle("GET /v1/models/{modelID}", HandlerModelByID(d.ModelsIndex))
	mux.Handle("PUT /v1/models/ei/projects/{projectID}", HandleInstallEIModel(d.Config, d.Models))
	mux.Handle("PUT /v1/models/{modelID}", HandleInstallModel(d.Models, d.ModelsIndex))
	mux.Handle("POST /v1/models", HandleDownloadModel(d.Models))
	mux.Handle("DELETE /v1/models/{modelID}", HandlerDeleteModelByID(d.Models))

	mux.Handle("GET /v1/apps", HandleAppList(d.Runtime, d.IDProvider, d.BricksIndex, d.Config, d.Platform))
	mux.Handle("POST /v1/apps", HandleAppCreate(d.BricksIndex, d.IDProvider, d.Config))
	mux.Handle("GET /v1/apps/{appID}", HandleAppDetails(d.Runtime, d.BricksIndex, d.IDProvider, d.Config))
	mux.Handle("PATCH /v1/apps/{appID}", HandleAppDetailsEdits(d.Runtime, d.BricksIndex, d.IDProvider, d.Config))
	mux.Handle("POST /v1/apps/{appID}/clone", HandleAppClone(d.IDProvider, d.Config))
	mux.Handle("DELETE /v1/apps/{appID}", HandleAppDelete(d.Runtime, d.IDProvider, d.Platform, d.Config))
	mux.Handle("GET /v1/apps/{appID}/export", HandleAppExport(d.IDProvider, d.BricksIndex))
	mux.Handle("POST /v1/apps/import", HandleAppImport(d.Config, d.IDProvider))
	mux.Handle("GET /v1/apps/{appID}/exposed-ports", HandleAppPorts(d.BricksIndex, d.ServicesIndex, d.IDProvider))
	mux.Handle("PUT /v1/apps/{appID}/sketch/libraries/{libRef}", HandleSketchAddLibrary(d.IDProvider))
	mux.Handle("DELETE /v1/apps/{appID}/sketch/libraries/{libRef}", HandleSketchRemoveLibrary(d.IDProvider))
	mux.Handle("GET /v1/apps/{appID}/sketch/libraries", HandleSketchListLibraries(d.IDProvider))

	mux.Handle("GET /v1/apps/{appID}/bricks", HandleAppBrickInstancesList(d.BrickService, d.IDProvider))
	mux.Handle("GET /v1/apps/{appID}/bricks/{brickID}", HandleAppBrickInstanceDetails(d.BrickService, d.IDProvider))
	mux.Handle("PUT /v1/apps/{appID}/bricks/{brickID}", HandleBrickCreate(d.BrickService, d.IDProvider))
	mux.Handle("PATCH /v1/apps/{appID}/bricks/{brickID}", HandleBrickUpdates(d.BrickService, d.IDProvider))
	mux.Handle("DELETE /v1/apps/{appID}/bricks/{brickID}", HandleBrickDelete(d.BrickService, d.IDProvider))
	mux.Handle("POST /v1/apps/{appID}/bricks", HandleAppLocalBrickCreate(d.IDProvider))
	mux.Handle("POST /v1/apps/{appID}/bricks/{brickID}/rename", HandleAppLocalBrickRename(d.BrickService, d.IDProvider))

	mux.Handle("GET /v1/libraries", HandleLibraryList(d.Config.LibrariesAPIURL, d.Version))
}
