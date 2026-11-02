// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package handlers

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/arduino/arduino-app-cli/internal/api/models"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appid"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appstore"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/platform"
	"github.com/arduino/arduino-app-cli/internal/render"
)

type AppListResponse struct {
	Apps       []appstore.AppInfo       `json:"apps" description:"List of applications"`
	BrokenApps []appstore.BrokenAppInfo `json:"broken_apps,omitempty" description:"List of applications that are broken and couldn't be parsed"`
}

func HandleAppList(
	rt appstore.Runtime,
	idProvider *appid.Provider,
	bricksIndex *bricksindex.BricksIndex,
	cfg config.Configuration,
	platform platform.Platform,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()

		showExamples, showApps, showOnlyDefault, showReleases := true, true, false, true
		if filter := queryParams.Get("filter"); filter != "" {
			filters := strings.Split(strings.TrimSpace(filter), ",")
			showExamples = slices.Contains(filters, "examples")
			showOnlyDefault = slices.Contains(filters, "default")
			showApps = slices.Contains(filters, "apps")
			showReleases = slices.Contains(filters, "releases")
		}

		var statusFilter appstore.Status
		if status := queryParams.Get("status"); status != "" {
			status, err := appstore.ParseStatus(status)
			if err != nil {
				render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: "invalid status filter"})
				return
			}
			statusFilter = status
		}

		res, err := appstore.ListApps(r.Context(), rt, appstore.ListAppRequest{
			ShowApps:        showApps,
			ShowExamples:    showExamples,
			ShowReleases:    showReleases,
			ShowOnlyDefault: showOnlyDefault,
			StatusFilter:    statusFilter,
		}, idProvider, bricksIndex, cfg, platform)
		if err != nil {
			slog.Error("Unable to parse the app.yaml", slog.String("error", err.Error()))
			render.EncodeResponse(w, http.StatusInternalServerError, models.ErrorResponse{Details: "unable to find the app"})

			return
		}
		render.EncodeResponse(w, http.StatusOK, AppListResponse{Apps: res.Apps, BrokenApps: res.BrokenApps})
	}
}
