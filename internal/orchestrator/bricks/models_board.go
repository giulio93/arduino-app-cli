// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package bricks

import (
	"context"

	"go.bug.st/f"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
)

// BoardModels answers the service from a models index. An index with no Runner (boardless
// mode) answers from its catalog.
func BoardModels(idx *modelsindex.ModelsIndex) Models { return boardModels{idx} }

type boardModels struct{ idx *modelsindex.ModelsIndex }

func (b boardModels) ByBrick(ctx context.Context, brickID string) ([]AIModel, error) {
	matches, err := b.idx.NewLookup().ByBrick(ctx, brickID)
	return f.Map(matches, func(m modelsindex.AIModelLite) AIModel {
		// TODO: Description is deprecated, remove in future versions
		return AIModel{ID: m.ID, Name: m.Name, Description: m.Description}
	}), err
}

func (b boardModels) ModelForBrick(ctx context.Context, modelID, brickID string) (string, error) {
	model, err := b.idx.NewLookup().ModelForBrick(ctx, modelID, brickID)
	if err != nil || model == nil {
		return "", err
	}
	return model.ID, nil
}
