// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package orchestrator

import "github.com/arduino/arduino-app-cli/internal/orchestrator/appstore"

// The app store moved to package appstore, which has no docker and builds for js. These
// aliases keep the callers of orchestrator compiling; new code uses appstore.

type (
	AIModelsListRequest = appstore.AIModelsListRequest
	AppDetailedBrick    = appstore.AppDetailedBrick
	AppDetailedInfo     = appstore.AppDetailedInfo
	AppEditRequest      = appstore.AppEditRequest
	AppInfo             = appstore.AppInfo
	AppStatusInfo       = appstore.AppStatusInfo
	BrickExamples       = appstore.BrickExamples
	BrokenAppInfo       = appstore.BrokenAppInfo
	CategoryExamples    = appstore.CategoryExamples
	CloneAppRequest     = appstore.CloneAppRequest
	CloneAppResponse    = appstore.CloneAppResponse
	ConfigDirectories   = appstore.ConfigDirectories
	ConfigResponse      = appstore.ConfigResponse
	CreateAppRequest    = appstore.CreateAppRequest
	CreateAppResponse   = appstore.CreateAppResponse
	Example             = appstore.Example
	ExampleResponse     = appstore.ExampleResponse
	LibraryReleaseID    = appstore.LibraryReleaseID
	ListAppRequest      = appstore.ListAppRequest
	ListAppResult       = appstore.ListAppResult
	PortInfo            = appstore.PortInfo
	RequiredService     = appstore.RequiredService
	Runtime             = appstore.Runtime
	Status              = appstore.Status
)

const (
	StatusStarting      = appstore.StatusStarting
	StatusRunning       = appstore.StatusRunning
	StatusStopping      = appstore.StatusStopping
	StatusStopped       = appstore.StatusStopped
	StatusFailed        = appstore.StatusFailed
	StatusUninitialized = appstore.StatusUninitialized
)

var (
	ErrAppAlreadyExists   = appstore.ErrAppAlreadyExists
	ErrAppDoesntExists    = appstore.ErrAppDoesntExists
	ErrAppNotFound        = appstore.ErrAppNotFound
	ErrBadRequest         = appstore.ErrBadRequest
	ErrNotPrepared        = appstore.ErrNotPrepared
	ErrAppRunning         = appstore.ErrAppRunning
	AIModelDetails        = appstore.AIModelDetails
	AIModelsList          = appstore.AIModelsList
	DeleteApp             = appstore.DeleteApp
	EditApp               = appstore.EditApp
	ExportAppZip          = appstore.ExportAppZip
	GetDefaultApp         = appstore.GetDefaultApp
	ImportAppFromZip      = appstore.ImportAppFromZip
	SetArduinoCliConfig   = appstore.SetArduinoCliConfig
	SetDefaultApp         = appstore.SetDefaultApp
	AppDetails            = appstore.AppDetails
	CloneApp              = appstore.CloneApp
	GetOrchestratorConfig = appstore.GetOrchestratorConfig
	CreateApp             = appstore.CreateApp
	GetExamples           = appstore.GetExamples
	AddSketchLibrary      = appstore.AddSketchLibrary
	ListSketchLibraries   = appstore.ListSketchLibraries
	NewLibraryReleaseID   = appstore.NewLibraryReleaseID
	ParseLibraryReleaseID = appstore.ParseLibraryReleaseID
	RemoveSketchLibrary   = appstore.RemoveSketchLibrary
	ListApps              = appstore.ListApps
	AppPorts              = appstore.AppPorts
	RequiredServices      = appstore.RequiredServices
	ParseStatus           = appstore.ParseStatus

	appSourceFilter = appstore.AppSourceFilter
	redactSecrets   = appstore.RedactSecrets
)

const appPortsSource = appstore.AppPortsSource
