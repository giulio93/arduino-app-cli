// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package modelsindex

import (
	"context"
	"io"
)

// Runner runs a model handler's container to completion, as `docker run --rm` does. The
// board passes dockerrunner.New; a nil Runner means no handler can run (boardless mode),
// so the index answers its catalog only and install, download and delete fail.
type Runner interface {
	Run(ctx context.Context, opts RunOptions) error
}

// RunOptions is one handler run: the image, its command, its mounts and environment, and
// where its output goes.
type RunOptions struct {
	Image  string
	Cmd    []string
	Binds  []string
	Env    map[string]string
	Stdout io.Writer
	Stderr io.Writer
}
