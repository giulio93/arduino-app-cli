// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import (
	"cmp"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/app"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/servicesindex"
)

// AppPortsSource is the collision source name used for the ports declared in the app.yaml file.
const AppPortsSource = "app.yaml"

// RequiredService is a singleton service pulled in by an app brick. The brick that required it is
// kept around because services are not a concept exposed to the user: ports and collisions are
// reported against the brick, not against the service.
type RequiredService struct {
	ID      string
	Name    string
	BrickID string
	Ports   []string
}

// RequiredServices returns the services required by the given app bricks.
func RequiredServices(
	bricks []app.Brick,
	index *bricksindex.BricksIndex,
	servicesIndex *servicesindex.ServicesIndex,
) ([]RequiredService, error) {
	services := make(map[string]RequiredService)
	for _, appBrick := range bricks {
		indexBrick, found := index.FindBrickByID(appBrick.ID)
		if !found {
			return nil, fmt.Errorf("brick %q not found in the index", appBrick.ID)
		}

		matchingServices, err := indexBrick.GetMatchingService(bricksindex.BrickInstance{
			Model: cmp.Or(appBrick.Model, indexBrick.ModelName),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get required services for brick %s: %w", appBrick.ID, err)
		}

		for _, id := range matchingServices {
			if _, alreadyRequired := services[id]; alreadyRequired {
				continue
			}
			service, found := servicesIndex.FindServiceByID(id)
			if !found {
				slog.Debug("service required by brick not found or not available for current board", slog.String("service_id", id), slog.String("brick_id", appBrick.ID))
				continue
			}
			services[id] = RequiredService{ID: id, Name: service.Name, BrickID: appBrick.ID, Ports: service.GetPorts()}
		}
	}

	return slices.SortedFunc(maps.Values(services), func(a, b RequiredService) int {
		return cmp.Compare(a.ID, b.ID)
	}), nil
}
