// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appid"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/platform"
	"github.com/arduino/go-paths-helper"
)

type ListAppResult struct {
	Apps       []AppInfo       `json:"apps"`
	BrokenApps []BrokenAppInfo `json:"broken_apps"`
}

type AppInfo struct {
	ID          appid.ID `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Status      Status   `json:"status,omitempty"`
	Example     bool     `json:"example"`
	Default     bool     `json:"default"`
	// Release tells an app installed from a release, which is frozen, from an app,
	// which is edited. ReleaseID is the release it comes from.
	Release      bool       `json:"release"`
	ReleaseID    string     `json:"release_id,omitempty"`
	ReleaseLabel string     `json:"release_label,omitempty"`
	CreatedAt    *time.Time `json:"created_at,omitempty"`
}

type BrokenAppInfo struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type ListAppRequest struct {
	ShowExamples    bool
	ShowOnlyDefault bool // List only the default app (runs at startup)
	ShowApps        bool
	ShowReleases    bool // List the apps installed from a release, which are read-only
	StatusFilter    Status
}

func ListApps(
	ctx context.Context,
	rt Runtime,
	req ListAppRequest,
	idProvider *appid.Provider,
	bricksIndex *bricksindex.BricksIndex,
	cfg config.Configuration,
	platform platform.Platform,
) (ListAppResult, error) {
	// Get the default app to mark it in the list
	defaultApp, err := GetDefaultApp(cfg)
	if err != nil {
		slog.Warn("unable to get default app", slog.String("error", err.Error()))
	}

	// Get the status of all apps (it will return only the apps that have been started at least once)
	var appsStatus []AppStatusInfo
	if rt != nil {
		appsStatus, err = rt.AppsStatus(ctx)
		if err != nil {
			slog.Error("unable to get running app", slog.String("error", err.Error()))
		}
	}

	// Retrieve all apps from the filesystem
	var pathsToExplore paths.PathList
	if req.ShowExamples || req.ShowOnlyDefault {
		pathsToExplore.AddAll(cfg.ExamplesDirs(platform))
		pathsToExplore.AddAll(cfg.ExamplesAdditionalDirs())
	}
	if req.ShowApps || req.ShowOnlyDefault {
		pathsToExplore.Add(cfg.AppsDir())
	}
	// The releases are apps, installed apart because they are read-only.
	if req.ShowReleases || req.ShowOnlyDefault {
		pathsToExplore.Add(cfg.ReleasesDir())
	}

	appPaths, err := app.FindAppsInFolders(pathsToExplore)
	if err != nil {
		slog.Error("unable to list apps", slog.String("error", err.Error()))
		return ListAppResult{}, err
	}

	// Go through all the known app paths, and create a result item for each.
	result := ListAppResult{Apps: []AppInfo{}, BrokenApps: []BrokenAppInfo{}}
	for _, file := range appPaths {
		app, err := app.Load(file)
		if err != nil {
			result.BrokenApps = append(result.BrokenApps, BrokenAppInfo{
				Name:  file.Base(),
				Error: fmt.Sprintf("unable to parse the app.yaml: %s", err.Error()),
			})
			continue
		}

		// Apply default-apps-only filter if requested
		isDefault := defaultApp != nil && defaultApp.FullPath.EqualsTo(app.FullPath)
		if req.ShowOnlyDefault && !isDefault {
			continue
		}

		// Retrieve the app status if available
		var status Status
		if rt != nil {
			status = StatusUninitialized
		}
		if idx := slices.IndexFunc(appsStatus, func(a AppStatusInfo) bool {
			return a.AppPath.EqualsTo(app.FullPath)
		}); idx != -1 {
			status = appsStatus[idx].Status
		}

		// Apply status filter if requested
		if req.StatusFilter != "" && req.StatusFilter != status {
			continue
		}

		// Get the app ID
		id, err := idProvider.IDFromPath(app.FullPath)
		if err != nil {
			return ListAppResult{}, fmt.Errorf("failed to get app ID from path %s: %w", file.String(), err)
		}

		// Filter example apps that use bricks incompatible with the board
		if bricksIndex != nil && id.IsExample() && !exampleCompatibleWithBricksIndex(app, bricksIndex) {
			continue
		}

		release, isRelease := app.GetRelease()

		result.Apps = append(result.Apps,
			AppInfo{
				ID:           id,
				Name:         app.Name,
				Description:  app.Descriptor.Description,
				Icon:         app.Descriptor.Icon,
				Status:       status,
				Example:      id.IsExample(),
				Default:      isDefault,
				Release:      isRelease,
				ReleaseID:    release.ID,
				ReleaseLabel: release.ReleaseLabel,
				CreatedAt:    release.CreatedAt,
			},
		)
	}

	return result, nil
}

// exampleCompatibleWithBricksIndex returns true if all built-in bricks referenced by the app
// are present in the given bricks index. Local bricks bundled with the app are always treated
// as compatible and are skipped in the check.
func exampleCompatibleWithBricksIndex(a app.ArduinoApp, idx *bricksindex.BricksIndex) bool {
	localBrickIDs := make(map[string]struct{}, len(a.LocalBricks))
	for _, lb := range a.LocalBricks {
		localBrickIDs[lb.ID] = struct{}{}
	}
	for _, brick := range a.Descriptor.Bricks {
		if _, isLocal := localBrickIDs[brick.ID]; isLocal {
			continue
		}
		if _, ok := idx.FindBrickByID(brick.ID); !ok {
			return false
		}
	}
	return true
}
