// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import "github.com/arduino/arduino-app-cli/internal/orchestrator/config"

type ConfigResponse struct {
	Directories  ConfigDirectories `json:"directories"`
	PythonRunner string            `json:"python_runner"`
}

type ConfigDirectories struct {
	Data     string `json:"data"`
	Apps     string `json:"apps"`
	Examples string `json:"examples"`
}

func GetOrchestratorConfig(cfg config.Configuration) ConfigResponse {
	return ConfigResponse{
		Directories: ConfigDirectories{
			Data: cfg.DataDir().String(),
			Apps: cfg.AppsDir().String(),
			// FIXME: ExamplesDir() is not the right directory, we should return the examples by board directory.
			// Examples: cfg.ExamplesDir().String(),
		},
		PythonRunner: cfg.RunnerVersion,
	}
}
