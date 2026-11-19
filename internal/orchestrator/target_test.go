// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

func TestCheckTarget(t *testing.T) {
	unoq, _ := platform.ForBoard(platform.BoardUnoQ)
	withTarget := func(target string) app.ArduinoApp {
		return app.ArduinoApp{Name: "a", Descriptor: app.AppDescriptor{Target: target}}
	}
	require.NoError(t, CheckTarget(withTarget(""), unoq))
	require.NoError(t, CheckTarget(withTarget(platform.BoardUnoQ), unoq))
	require.ErrorIs(t, CheckTarget(withTarget(platform.BoardVentunoQ), unoq), ErrTargetMismatch)
}
