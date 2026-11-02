// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package dockerrunner runs the model handlers on the board's docker engine.
package dockerrunner

import (
	"context"

	"github.com/moby/moby/client"

	"github.com/arduino/arduino-app-cli/internal/dockerhelper"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
)

type runner struct{ cli client.APIClient }

// New returns a modelsindex.Runner on cli. A nil cli gives a nil Runner.
func New(cli client.APIClient) modelsindex.Runner {
	if cli == nil {
		return nil
	}
	return runner{cli: cli}
}

func (r runner) Run(ctx context.Context, o modelsindex.RunOptions) error {
	return dockerhelper.Run(ctx, r.cli, dockerhelper.RunOptions{
		Image: o.Image, Cmd: o.Cmd, Binds: o.Binds, Env: o.Env, Stdout: o.Stdout, Stderr: o.Stderr,
	})
}
