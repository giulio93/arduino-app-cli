// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"embed"
	"net/http"

	"github.com/arduino/arduino-app-cli/internal/api/boardhandlers"
	"github.com/arduino/arduino-app-cli/internal/api/handlers"
	"github.com/arduino/arduino-app-cli/internal/orchestrator"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appid"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricks"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/servicesindex"
	"github.com/arduino/arduino-app-cli/internal/platform"
	"github.com/arduino/arduino-app-cli/internal/releasebuild"
	"github.com/arduino/arduino-app-cli/internal/update"

	"github.com/docker/cli/cli/command"

	_ "net/http/pprof" //nolint:gosec // pprof import is safe for profiling endpoints
)

//go:embed docs
var docsFS embed.FS

func NewHTTPRouter(
	dockerClient command.Cli,
	version string,
	updater *update.Manager,
	provisioner *orchestrator.Provision,
	modelsIndex *modelsindex.ModelsIndex,
	bricksIndex *bricksindex.BricksIndex,
	servicesIndex *servicesindex.ServicesIndex,
	brickService *bricks.Service,
	idProvider *appid.Provider,
	platform platform.Platform,
	cfg config.Configuration,
	buildEvents *releasebuild.EventBroker,
	allowedOrigins []string,
) http.Handler {
	appRuntime := orchestrator.NewDockerRuntime(dockerClient)
	mux := http.NewServeMux()
	mux.Handle("GET /debug/", http.DefaultServeMux) // pprof endpoints

	handlers.Register(mux, handlers.Deps{
		Version:       version,
		Config:        cfg,
		Platform:      platform,
		Runtime:       appRuntime,
		BricksIndex:   bricksIndex,
		ModelsIndex:   modelsIndex,
		ServicesIndex: servicesIndex,
		BrickService:  brickService,
		IDProvider:    idProvider,
		Models:        orchestrator.NewModelInstaller(dockerClient, cfg, modelsIndex, bricksIndex, idProvider, platform),
	})

	mux.Handle("GET /v1/system/update/check", handlers.HandleCheckUpgradable(updater))
	mux.Handle("GET /v1/system/update/events", handlers.HandleUpdateEvents(updater))
	mux.Handle("PUT /v1/system/update/apply", handlers.HandleUpdateApply(updater))
	mux.Handle("GET /v1/system/resources", handlers.HandleSystemResources(cfg))

	mux.Handle("GET /v1/apps/events", boardhandlers.HandlerAppStatus(dockerClient, idProvider, bricksIndex, cfg, platform))
	mux.Handle("GET /v1/apps/{appID}/logs", boardhandlers.HandleAppLogs(dockerClient, idProvider, bricksIndex, servicesIndex, cfg))
	mux.Handle("POST /v1/apps/{appID}/start", boardhandlers.HandleAppStart(dockerClient, provisioner, modelsIndex, bricksIndex, servicesIndex, idProvider, cfg, platform))
	mux.Handle("POST /v1/apps/{appID}/stop", boardhandlers.HandleAppStop(dockerClient, idProvider, platform, cfg))
	mux.Handle("POST /v1/apps/{appID}/build", boardhandlers.HandleAppBuild(dockerClient, provisioner, idProvider, cfg, buildEvents))
	mux.Handle("GET /v1/apps/build/events", boardhandlers.HandleAppBuildEvents(buildEvents))
	mux.Handle("PUT /v1/apps/install", boardhandlers.HandleAppInstall(dockerClient, provisioner, idProvider, cfg, platform))
	mux.Handle("PUT /v1/apps/{appID}/prepare", boardhandlers.HandleAppPrepare(dockerClient, provisioner, idProvider, cfg, platform))

	mux.Handle("GET /v1/docs/", http.StripPrefix("/v1/docs/", handlers.DocsServer(docsFS)))

	mux.Handle("GET /v1/monitor/ws", handlers.HandleMonitorWS(allowedOrigins))

	return mux
}
