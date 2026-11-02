// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package orchestrator

import (
	"context"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

// Runtime is what the app listing, details, edit and delete need from the place apps
// run. A nil Runtime means nothing can run (boardless mode): status is left empty,
// edit skips the running check and delete skips the stop.
type Runtime interface {
	AppsStatus(ctx context.Context) ([]AppStatusInfo, error)
	AppStatus(ctx context.Context, a app.ArduinoApp) (AppStatusInfo, error)
	StopAndDestroy(ctx context.Context, plat platform.Platform, a app.ArduinoApp, cfg config.Configuration) error
}
