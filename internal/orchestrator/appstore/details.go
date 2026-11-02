// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appid"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"go.bug.st/f"
)

type AppDetailedInfo struct {
	ID           appid.ID           `json:"id" required:"true" `
	Name         string             `json:"name" required:"true"`
	Path         string             `json:"path"`
	Description  string             `json:"description"`
	Icon         string             `json:"icon"`
	Status       Status             `json:"status" required:"true"`
	Example      bool               `json:"example"`
	Default      bool               `json:"default"`
	Release      bool               `json:"release"`
	ReleaseID    string             `json:"release_id,omitempty"`
	ReleaseLabel string             `json:"release_label,omitempty"`
	CreatedAt    *time.Time         `json:"created_at,omitempty"`
	Notes        string             `json:"notes,omitempty"`
	Bricks       []AppDetailedBrick `json:"bricks,omitempty"`
}

type AppDetailedBrick struct {
	ID           string `json:"id" required:"true"`
	Name         string `json:"name" required:"true"`
	Category     string `json:"category,omitempty"`
	RequireModel bool   `json:"require_model"`
}

func AppDetails(
	ctx context.Context,
	rt Runtime,
	userApp app.ArduinoApp,
	bricksIndex *bricksindex.BricksIndex,
	idProvider *appid.Provider,
	cfg config.Configuration,
) (AppDetailedInfo, error) {
	bricksIndex = bricksIndex.WithAppBricks(userApp.LocalBricks)
	var wg sync.WaitGroup
	wg.Add(2)
	var defaultAppPath string
	var status Status
	go func() {
		defer wg.Done()
		if rt == nil {
			return
		}
		app, err := rt.AppStatus(ctx, userApp)
		if err != nil {
			slog.Warn("unable to get app status", slog.String("error", err.Error()), slog.String("path", userApp.FullPath.String()))
			status = StatusStopped
		} else {
			status = app.Status
		}
	}()
	go func() {
		defer wg.Done()
		defaultApp, err := GetDefaultApp(cfg)
		if err != nil {
			slog.Warn("unable to get default app", slog.String("error", err.Error()))
			return
		}
		if defaultApp == nil {
			return
		}
		defaultAppPath = defaultApp.FullPath.String()

	}()
	wg.Wait()

	id, err := idProvider.IDFromPath(userApp.FullPath)
	if err != nil {
		return AppDetailedInfo{}, err
	}

	release, isRelease := userApp.GetRelease()

	return AppDetailedInfo{
		ID:           id,
		Name:         userApp.Name,
		Path:         userApp.FullPath.String(),
		Description:  userApp.Descriptor.Description,
		Icon:         userApp.Descriptor.Icon,
		Status:       status,
		Example:      id.IsExample(),
		Default:      defaultAppPath == userApp.FullPath.String(),
		Release:      isRelease,
		ReleaseID:    release.ID,
		ReleaseLabel: release.ReleaseLabel,
		CreatedAt:    release.CreatedAt,
		Notes:        release.Notes,
		Bricks: f.Map(userApp.Descriptor.Bricks, func(b app.Brick) AppDetailedBrick {
			res := AppDetailedBrick{ID: b.ID}
			bi, found := bricksIndex.FindBrickByID(b.ID)
			if !found {
				slog.Warn("brick not found in bricks index", slog.String("id", b.ID), slog.String("app", userApp.FullPath.String()))
				return res
			}
			res.Name = bi.Name
			res.Category = bi.Category
			res.RequireModel = bi.RequireModel
			return res
		}),
	}, nil
}
