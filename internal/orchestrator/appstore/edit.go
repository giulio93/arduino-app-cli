// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/arduino/arduino-app-cli/internal/fatomic"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/go-paths-helper"
	"github.com/gosimple/slug"
)

type AppEditRequest struct {
	Name        *string
	Icon        *string
	Description *string
	Default     *bool
}

func EditApp(
	ctx context.Context,
	rt Runtime,
	req AppEditRequest,
	editApp *app.ArduinoApp,
	cfg config.Configuration,
) (editErr error) {
	if req.Name != nil && slug.Make(*req.Name) == "" {
		return fmt.Errorf("%w: invalid app name %q", app.ErrInvalidApp, *req.Name)
	}

	if req.Name != nil && rt != nil {
		// Containers keep the old app path in their labels: renaming a running app breaks them.
		status, err := rt.AppStatus(ctx, *editApp)
		if err != nil {
			return fmt.Errorf("failed to get app status: %w", err)
		}
		if status.Status == StatusRunning || status.Status == StatusStarting || status.Status == StatusStopping {
			return ErrAppRunning
		}
	}

	if req.Name != nil || req.Icon != nil || req.Description != nil {
		if err := editAppFolder(req, editApp, cfg); err != nil {
			return err
		}
	}

	if req.Default != nil {
		if err := editAppDefaults(editApp, *req.Default, cfg); err != nil {
			return fmt.Errorf("failed to edit app defaults: %w", err)
		}
	}
	return nil
}

func editAppFolder(req AppEditRequest, editApp *app.ArduinoApp, cfg config.Configuration) error {
	editable, err := editApp.GetAsEditable()
	if err != nil {
		return err
	}

	if req.Name != nil {
		editable.Descriptor.Name = *req.Name
	}
	if req.Icon != nil {
		editable.Descriptor.Icon = *req.Icon
	}
	if req.Description != nil {
		editable.Descriptor.Description = *req.Description
	}

	if err := editable.Descriptor.IsValid(); err != nil {
		return fmt.Errorf("%w: %w", app.ErrInvalidApp, err)
	}

	if req.Name != nil {
		newPath, err := findRenamePath(editable.FullPath, slug.Make(*req.Name))
		if err != nil {
			return err
		}
		if !newPath.EqualsTo(editable.FullPath) {
			oldPath := editable.FullPath
			if err := editable.FullPath.Rename(newPath); err != nil {
				return fmt.Errorf("failed to rename app path: %w", err)
			}
			fatomic.Sync()
			editable.FullPath = newPath
			// The folder is already renamed: the edit must complete, so the client gets the new ID.
			if err := moveDefaultApp(oldPath, editApp, cfg); err != nil {
				slog.Warn("failed to update default app after renaming", slog.String("path", newPath.String()), slog.String("error", err.Error()))
			}
		}
		editable.Name = editable.Descriptor.Name
	}

	return editable.Save()
}

func findRenamePath(appPath *paths.Path, folderName string) (*paths.Path, error) {
	candidate := appPath.Parent().Join(folderName)
	for i := 1; i <= 100; i++ {
		if candidate.EqualsTo(appPath) || candidate.NotExist() {
			return candidate, nil
		}
		candidate = appPath.Parent().Join(fmt.Sprintf("%s-%d", folderName, i))
	}
	return nil, ErrAppAlreadyExists
}

func moveDefaultApp(oldPath *paths.Path, movedApp *app.ArduinoApp, cfg config.Configuration) error {
	defaultAppFilePath := cfg.DataDir().Join(defaultAppFileName)
	if defaultAppFilePath.NotExist() {
		return nil
	}
	defaultAppPath, err := defaultAppFilePath.ReadFile()
	if err != nil {
		return err
	}
	if string(bytes.TrimSpace(defaultAppPath)) != oldPath.String() {
		return nil
	}
	return SetDefaultApp(movedApp, cfg)
}

func editAppDefaults(userApp *app.ArduinoApp, isDefault bool, cfg config.Configuration) error {
	if isDefault {
		if err := SetDefaultApp(userApp, cfg); err != nil {
			return fmt.Errorf("failed to set default app: %w", err)
		}
		return nil
	}

	defaultApp, err := GetDefaultApp(cfg)
	if err != nil {
		return fmt.Errorf("failed to get default app: %w", err)
	}

	// No default app set, nothing to unset.
	if defaultApp == nil {
		return nil
	}

	// Unset only if the current default is the same as the app being edited.
	if defaultApp.FullPath.String() == userApp.FullPath.String() {
		if err := SetDefaultApp(nil, cfg); err != nil {
			return fmt.Errorf("failed to unset default app: %w", err)
		}
	}
	return nil
}
