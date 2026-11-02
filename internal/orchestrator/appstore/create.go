// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import (
	"fmt"

	"github.com/arduino/arduino-app-cli/internal/fatomic"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	appgenerator "github.com/arduino/arduino-app-cli/internal/orchestrator/app/generator"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appid"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/go-paths-helper"
	"github.com/gosimple/slug"
	"gopkg.in/yaml.v2"
)

type CreateAppRequest struct {
	Name        string
	Icon        string
	Description string
	Bricks      []string
	SkipSketch  bool
}

type CreateAppResponse struct {
	ID appid.ID `json:"id"`
}

func CreateApp(
	req CreateAppRequest,
	bricksIndex *bricksindex.BricksIndex,
	idProvider *appid.Provider,
	cfg config.Configuration,
) (CreateAppResponse, error) {
	if req.Name == "" {
		return CreateAppResponse{}, fmt.Errorf("app name cannot be empty")
	}

	appBricks := make([]app.Brick, 0, len(req.Bricks))
	for _, id := range req.Bricks {
		if _, found := bricksIndex.FindBrickByID(id); !found {
			return CreateAppResponse{}, fmt.Errorf("%w: brick %q not found", ErrBadRequest, id)
		}
		appBricks = append(appBricks, app.Brick{ID: id})
	}

	basePath, appExists := findAppPathByName(req.Name, cfg)
	if appExists {
		existingID, err := idProvider.IDFromPath(basePath)
		if err != nil {
			return CreateAppResponse{}, ErrAppAlreadyExists
		}
		return CreateAppResponse{}, fmt.Errorf("%w with id: %q", ErrAppAlreadyExists, existingID)
	}
	appName := req.Name
	newApp := app.AppDescriptor{
		Name:        appName,
		Description: req.Description,
		Ports:       []int{},
		Bricks:      appBricks,
		Icon:        req.Icon,
	}
	if err := newApp.IsValid(); err != nil {
		return CreateAppResponse{}, fmt.Errorf("%w: %v", app.ErrInvalidApp, err)
	}

	if err := appgenerator.GenerateApp(basePath, newApp, req.SkipSketch); err != nil {
		return CreateAppResponse{}, fmt.Errorf("failed to create app: %w", err)
	}
	fatomic.Sync()
	id, err := idProvider.IDFromPath(basePath)
	if err != nil {
		return CreateAppResponse{}, fmt.Errorf("failed to get app id: %w", err)
	}
	return CreateAppResponse{ID: id}, nil
}

type CloneAppRequest struct {
	FromID appid.ID

	Name *string
	Icon *string
}

type CloneAppResponse struct {
	ID appid.ID `json:"id"`
}

func CloneApp(
	req CloneAppRequest,
	idProvider *appid.Provider,
	cfg config.Configuration,
) (response CloneAppResponse, cloneErr error) {
	originPath := req.FromID.ToPath()
	if !originPath.Exist() {
		return CloneAppResponse{}, ErrAppDoesntExists
	}
	if !originPath.Join("app.yaml").Exist() {
		return CloneAppResponse{}, app.ErrInvalidApp
	}

	var dstPath *paths.Path
	if req.Name != nil && *req.Name != "" {
		dstPath = cfg.AppsDir().Join(slug.Make(*req.Name))
		if dstPath.Exist() {
			return CloneAppResponse{}, ErrAppAlreadyExists
		}
	} else {
		for i := range 100 { // In case of name collision, we try up to 100 times.
			dstName := fmt.Sprintf("%s-copy%d", originPath.Base(), i)
			dstPath = cfg.AppsDir().Join(dstName)
			if !dstPath.Exist() {
				break
			}
		}
	}
	if err := dstPath.MkdirAll(); err != nil {
		return CloneAppResponse{}, fmt.Errorf("failed to create app directory: %w", err)
	}

	// In case something during the clone operation fails we remove the dst path
	defer func() {
		if cloneErr != nil {
			_ = dstPath.RemoveAll()
		}
	}()

	// The manifest stays behind: a copy of a release is an app, and is edited.
	list, err := originPath.ReadDir(paths.FilterOutNames(".cache", "data", app.ReleaseManifestFileName))
	if err != nil {
		return CloneAppResponse{}, fmt.Errorf("failed to read app directory: %w", err)
	}
	for _, file := range list {
		if file.IsDir() {
			if err := file.CopyDirTo(dstPath.Join(file.Base())); err != nil {
				return CloneAppResponse{}, fmt.Errorf("failed to copy directory: %w", err)
			}
		} else {
			if err := file.CopyTo(dstPath.Join(file.Base())); err != nil {
				return CloneAppResponse{}, fmt.Errorf("failed to copy file: %w", err)
			}
		}
	}

	if (req.Name != nil && *req.Name != "") || (req.Icon != nil && *req.Icon != "") {

		appYamlPath := dstPath.Join("app.yaml")
		if !appYamlPath.Exist() {
			return CloneAppResponse{}, fmt.Errorf("app.yaml not found in cloned app")
		}
		descriptor, err := app.ParseDescriptorFile(appYamlPath)
		if err != nil {
			return CloneAppResponse{}, fmt.Errorf("failed to parse app.yaml file: %w", err)
		}
		if req.Name != nil && *req.Name != "" {
			descriptor.Name = *req.Name
		}
		if req.Icon != nil && *req.Icon != "" {
			descriptor.Icon = *req.Icon
		}

		// TODO: implement MarshalYaml directly in the descriptor.
		newDescriptor, err := yaml.Marshal(descriptor)
		if err != nil {
			// TODO: should we consider this a fatal error, or we prefer to silently ignore the error?
			// Worst case, the optional fields will be the same as the source app.
			return CloneAppResponse{}, fmt.Errorf("failed to marshal app.yaml file: %w", err)
		}
		if err := appYamlPath.WriteFile(newDescriptor); err != nil {
			return CloneAppResponse{}, fmt.Errorf("failed to write app.yaml file: %w", err)
		}
	}
	fatomic.Sync()

	id, err := idProvider.IDFromPath(dstPath)
	if err != nil {
		return CloneAppResponse{}, fmt.Errorf("failed to get app id: %w", err)
	}
	return CloneAppResponse{ID: id}, nil
}
