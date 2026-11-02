// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package boardless serves the Boardless API: the routes of the board daemon that need
// no board, for a Host with none (App Lab desktop in-process, the PWA as WebAssembly).
// It has no docker in its import graph and builds for GOOS=js.
//
// App status is omitted. Installing, downloading or deleting a model, and the app events
// stream, reply 501 with the error code "needs_board"; the other board routes (start,
// stop, logs, ...) are absent.
package boardless

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-app-cli/internal/api/handlers"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appid"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricks"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/servicesindex"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

// Options is what the Host states besides the directories.
type Options struct {
	// Board is the Virtual board's type: "unoq" or "ventunoq". Empty means "unoq".
	Board string
	// Version is what GET /v1/version answers.
	Version string
}

// ErrUnknownBoard is returned for an Options.Board that is not a supported board.
var ErrUnknownBoard = errors.New("unknown board")

// NewHandler returns the Boardless API for o. The directories come from the environment
// variables the board daemon reads, which the Host sets (on js, through go.env):
//
//   - ARDUINO_APP_CLI__APPS_DIR: the Local workspace, where the apps live.
//   - ARDUINO_APP_CLI__DATA_DIR: properties, staging, the examples in DATA_DIR/examples
//     and the catalog in DATA_DIR/assets/<runner version>, as on a board.
//   - ARDUINO_APP_BRICKS__CUSTOM_MODEL_DIR: the custom models. Unset means $HOME/.arduino-bricks/models.
//
// The apps and data directories are created if missing. A missing or broken catalog is
// an error, not a panic.
func NewHandler(o Options) (http.Handler, error) {
	if o.Board == "" {
		o.Board = platform.BoardUnoQ
	}
	plat, ok := platform.ForBoard(o.Board)
	if !ok {
		return nil, fmt.Errorf("%w %q: supported are %v", ErrUnknownBoard, o.Board, platform.SupportedBoards())
	}
	cfg, err := config.NewFromEnv()
	if err != nil {
		return nil, err
	}
	for _, dir := range []*paths.Path{cfg.AppsDir(), cfg.DataDir()} {
		if err := dir.MkdirAll(); err != nil {
			return nil, fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	bricksIndex, err := bricksindex.Load(plat, cfg.AssetDir())
	if err != nil {
		return nil, fmt.Errorf("loading the bricks catalog: %w", err)
	}
	// No Runner: the index answers its catalog, and installs need a board.
	modelsIndex, err := modelsindex.Load(plat, cfg.AssetDir(), cfg.ModelsDir(), cfg.CustomModelsDir(), nil, cfg)
	if err != nil {
		return nil, fmt.Errorf("loading the models catalog: %w", err)
	}
	servicesIndex, err := servicesindex.Load(plat, cfg.AssetDir().Join("services"))
	if err != nil {
		return nil, fmt.Errorf("loading the services catalog: %w", err)
	}

	mux := http.NewServeMux()
	handlers.Register(mux, handlers.Deps{
		Version:       o.Version,
		Config:        cfg,
		Platform:      plat,
		Runtime:       nil, // nothing runs here
		BricksIndex:   bricksIndex,
		ModelsIndex:   modelsIndex,
		ServicesIndex: servicesIndex,
		BrickService:  bricks.NewService(bricks.BoardModels(modelsIndex), bricksIndex),
		IDProvider:    appid.NewAppProvider(cfg, plat),
	})
	for _, pattern := range needsBoardRoutes {
		mux.Handle(pattern, handlers.NeedsBoard())
	}
	return mux, nil
}

// needsBoardRoutes answer "needs a board": the app events stream would otherwise reach
// GET /v1/apps/{appID} as the app "events" and fail as an invalid id. The model installs
// answer it from their handlers, which get no ModelInstaller here.
var needsBoardRoutes = []string{
	"GET /v1/apps/events",
}
