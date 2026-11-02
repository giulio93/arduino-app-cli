// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package handlers

import (
	"net/http"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/appstore"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/render"
)

func HandleConfig(cfg config.Configuration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := appstore.GetOrchestratorConfig(cfg)
		render.EncodeResponse(w, http.StatusOK, cfg)
	}
}
