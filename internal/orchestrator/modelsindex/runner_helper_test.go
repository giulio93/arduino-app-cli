// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package modelsindex

import (
	"context"

	"github.com/arduino/arduino-app-cli/internal/dockerhelper"
	"github.com/moby/moby/client"
)

// testRunner is dockerrunner's adapter, which this package's tests cannot import.
type testRunner struct{ cli client.APIClient }

func (r testRunner) Run(ctx context.Context, o RunOptions) error {
	return dockerhelper.Run(ctx, r.cli, dockerhelper.RunOptions{
		Image: o.Image, Cmd: o.Cmd, Binds: o.Binds, Env: o.Env, Stdout: o.Stdout, Stderr: o.Stderr,
	})
}

func runnerFor(cli client.APIClient) Runner {
	if cli == nil {
		return nil
	}
	return testRunner{cli: cli}
}
