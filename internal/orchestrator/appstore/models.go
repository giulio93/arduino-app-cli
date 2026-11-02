// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import (
	"context"
	"slices"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
)

type AIModelsListRequest struct {
	FilterByBrickID []string
}

// AIModelsList answers every model, filtered by brick when the request names one. It runs
// one listing container, and fails when that fails: the install status of every model
// comes from there, so a list without it states the declaration's guess as fact.
func AIModelsList(ctx context.Context, req AIModelsListRequest, modelsIndex *modelsindex.ModelsIndex) ([]modelsindex.AIModel, error) {
	collection, err := modelsIndex.NewLookup().All(ctx)
	if err != nil {
		return nil, err
	}
	if len(req.FilterByBrickID) != 0 {
		collection = slices.DeleteFunc(collection, func(model modelsindex.AIModel) bool {
			return !slices.ContainsFunc(model.Bricks, func(brick modelsindex.BrickConfig) bool {
				return slices.Contains(req.FilterByBrickID, brick.ID)
			})
		})
	}
	return collection, nil
}

// AIModelDetails describes the model id names. It runs the listing container unless the
// model is installed by its declaration. The id is plain: the API decodes the path before
// calling in.
func AIModelDetails(ctx context.Context, modelsIndex *modelsindex.ModelsIndex, id string) (modelsindex.AIModel, bool, error) {
	model, err := modelsIndex.NewLookup().ByID(ctx, id)
	if err != nil {
		return modelsindex.AIModel{}, false, err
	}
	if model == nil {
		return modelsindex.AIModel{}, false, nil
	}
	return *model, true, nil
}
