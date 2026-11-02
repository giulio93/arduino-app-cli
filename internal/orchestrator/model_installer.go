// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package orchestrator

import (
	"context"

	"github.com/docker/cli/cli/command"

	"github.com/arduino/arduino-app-cli/internal/api/edgeimpulse"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appid"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/bricksindex"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

// ModelInstaller installs, downloads and deletes models with the board's docker. It is
// what the API's model routes take, so the handlers need no docker of their own.
type ModelInstaller struct {
	docker      command.Cli
	cfg         config.Configuration
	modelsIndex *modelsindex.ModelsIndex
	bricksIndex *bricksindex.BricksIndex
	idProvider  *appid.Provider
	plat        platform.Platform
}

func NewModelInstaller(docker command.Cli, cfg config.Configuration, modelsIndex *modelsindex.ModelsIndex, bricksIndex *bricksindex.BricksIndex, idProvider *appid.Provider, plat platform.Platform) *ModelInstaller {
	return &ModelInstaller{docker: docker, cfg: cfg, modelsIndex: modelsIndex, bricksIndex: bricksIndex, idProvider: idProvider, plat: plat}
}

func (m *ModelInstaller) Install(ctx context.Context, id string, publish func(modelsindex.StreamMessage)) (modelsindex.AIModel, error) {
	return AIModelInstall(ctx, m.docker, m.modelsIndex, m.plat, id, publish)
}

func (m *ModelInstaller) Download(ctx context.Context, modelURL, mmprojURL string, publish func(modelsindex.StreamMessage)) (modelsindex.AIModel, error) {
	return AIModelDownload(ctx, m.docker, m.modelsIndex, m.plat, modelURL, mmprojURL, publish)
}

func (m *ModelInstaller) InstallEI(ctx context.Context, eiClient *edgeimpulse.EIClient, projectID, impulseID int) (modelsindex.AIModel, error) {
	return InstallEIModel(ctx, m.bricksIndex, m.modelsIndex, m.docker, eiClient, m.cfg.EIModelsDir(), m.plat, projectID, impulseID)
}

func (m *ModelInstaller) Delete(ctx context.Context, id string, force bool) error {
	return AIModelDelete(ctx, m.docker, m.cfg, m.modelsIndex, m.bricksIndex, m.plat, id, m.idProvider, force)
}
