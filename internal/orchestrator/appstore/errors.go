// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import "fmt"

var (
	ErrAppAlreadyExists = fmt.Errorf("app already exists")
	ErrAppDoesntExists  = fmt.Errorf("app doesn't exist")
	ErrAppNotFound      = fmt.Errorf("app not found")
	ErrBadRequest       = fmt.Errorf("bad request")
	ErrNotPrepared      = fmt.Errorf("app is not prepared")
	ErrAppRunning       = fmt.Errorf("app is running")
)
