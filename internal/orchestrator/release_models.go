// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package orchestrator

import (
	"log/slog"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
	"github.com/arduino/arduino-app-cli/internal/platform"
	"github.com/docker/cli/cli/command"
)

// appModels is the models index to validate the app against: a release answers with the one
// its build froze, the only index aware of a custom model it ships, and an editable app
// with the board index this cli holds.
func appModels(a *app.ArduinoApp, board *modelsindex.ModelsIndex, docker command.Cli, cfg config.Configuration, plat platform.Platform) *modelsindex.ModelsIndex {
	if a.IsRelease() {
		frozen, err := releaseModels(a, docker, cfg, plat)
		if err != nil {
			slog.Warn("cannot read the models the release ships", slog.String("app", a.Name), slog.String("error", err.Error()))
		} else {
			return frozen
		}
	}
	return board
}

// releaseModels is the models index a release ships in its .cache, the ones its build
// wired the app with.
func releaseModels(a *app.ArduinoApp, docker command.Cli, cfg config.Configuration, plat platform.Platform) (*modelsindex.ModelsIndex, error) {
	return modelsindex.Load(
		plat,
		a.ProvisioningStateDir(),
		cfg.ModelsDir(),
		cfg.CustomModelsDir(),
		docker.Client(),
		cfg,
	)
}
