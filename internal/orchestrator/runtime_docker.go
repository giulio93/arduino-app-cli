// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package orchestrator

import (
	"context"

	"github.com/docker/cli/cli/command"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

type dockerRuntime struct {
	cli command.Cli
}

// NewDockerRuntime returns the Runtime of a board, where apps run as containers.
func NewDockerRuntime(cli command.Cli) Runtime {
	return dockerRuntime{cli: cli}
}

func (d dockerRuntime) AppsStatus(ctx context.Context) ([]AppStatusInfo, error) {
	return getAppsStatus(ctx, d.cli.Client())
}

func (d dockerRuntime) AppStatus(ctx context.Context, a app.ArduinoApp) (AppStatusInfo, error) {
	return getAppStatus(ctx, d.cli.Client(), a)
}

func (d dockerRuntime) StopAndDestroy(ctx context.Context, plat platform.Platform, a app.ArduinoApp, cfg config.Configuration) error {
	return StopAndDestroyApp(ctx, d.cli, plat, a, cfg, func(StreamMessage) {})
}
