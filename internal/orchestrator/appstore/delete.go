// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import (
	"context"

	"github.com/arduino/arduino-app-cli/internal/fatomic"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

func DeleteApp(ctx context.Context, rt Runtime, platform platform.Platform, app app.ArduinoApp, cfg config.Configuration) error {
	// We try to remove docker related resources at best effort
	if rt != nil {
		_ = rt.StopAndDestroy(ctx, platform, app, cfg)
		// TODO: Shall we report stop error?
	}

	if err := app.FullPath.RemoveAll(); err != nil {
		return err
	}
	fatomic.Sync()
	return nil
}
