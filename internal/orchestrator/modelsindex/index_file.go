// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package modelsindex

import (
	"context"
	"fmt"

	"github.com/arduino/go-paths-helper"
	"github.com/goccy/go-yaml"
	"golang.org/x/sync/singleflight"
)

// The models index is <models dir>/.models-index.yaml. The listing container writes it, and
// every download and delete rewrites it before its container exits, so it is read on every
// lookup and nothing is kept between requests. It has the models-list.yaml shape, with what
// only the disk can tell added to each model.
const modelsIndexFileName = ".models-index.yaml"

type indexDocument struct {
	Models []map[string]indexEntry `yaml:"models"`
}

type indexEntry struct {
	AIModel   `yaml:",inline"`
	Status    ModelStatus `yaml:"status"`
	SizeBytes uint64      `yaml:"size_bytes"`
	Folder    string      `yaml:"folder"` // relative to the models dir
	Origin    ModelOrigin `yaml:"origin"`
}

// readIndex answers the models the index lists; found is false until a listing wrote it.
func readIndex(modelsDir *paths.Path) (models []AIModel, found bool, err error) {
	if modelsDir == nil {
		return nil, false, nil
	}
	file := modelsDir.Join(modelsIndexFileName)
	if file.NotExist() {
		return nil, false, nil
	}
	content, err := file.ReadFile()
	if err != nil {
		return nil, false, err
	}
	var doc indexDocument
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, false, fmt.Errorf("%s: %w", file, err)
	}
	models = make([]AIModel, 0, len(doc.Models))
	for _, item := range doc.Models {
		for id, entry := range item {
			model := entry.AIModel
			model.ID = id
			model.Status = entry.Status
			model.SizeBytes = entry.SizeBytes
			model.Origin = entry.Origin
			model.Preinstalled = model.Origin == CuratedOrigin && (model.Deployment == nil || model.Deployment.PreLoaded)
			if entry.Folder != "" {
				model.ModelFolderPath = modelsDir.Join(entry.Folder)
			}
			models = append(models, model)
		}
	}
	return models, true, nil
}

// listingRuns makes the callers that find no index share one listing container per models dir.
var listingRuns singleflight.Group

// runListing runs the listing container, which rewrites the index. It is not canceled with
// ctx: other callers may be waiting on the same run.
func (m *ModelsIndex) runListing(ctx context.Context) error {
	if m.Handlers == nil || m.Handlers.listing == nil || m.cli == nil {
		return nil
	}
	_, err, _ := listingRuns.Do(m.modelsDir.String(), func() (any, error) {
		return nil, runListAction(context.WithoutCancel(ctx), m.cli, m.Handlers.listing, m.Handlers.configEnv)
	})
	return err
}
