package parser

import (
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	appFolderPath := paths.New("testdata", "AppSimple")
	mainPythonPath := appFolderPath.Join("python", "main.py")
	mainSketchPath := appFolderPath.Join("sketch", "AppSimple.ino")

	app, err := Load("")
	assert.Empty(t, app)
	assert.Error(t, err)

	// Load app
	app, err = Load(appFolderPath.String())
	assert.NoError(t, err)
	assert.True(t, mainPythonPath.EquivalentTo(app.MainPythonFile))
	assert.True(t, mainSketchPath.EquivalentTo(app.MainSketchFile))
	assert.True(t, appFolderPath.EquivalentTo(app.FullPath))
}

func TestMissingDescriptor(t *testing.T) {
	appFolderPath := paths.New("testdata", "MissingDescriptor")

	// Load app
	app, err := Load(appFolderPath.String())
	assert.Error(t, err)
	assert.Empty(t, app)
	assert.ErrorContains(t, err, "descriptor app.yaml file missing from app")
}

func TestMissingMains(t *testing.T) {
	appFolderPath := paths.New("testdata", "MissingMains")

	// Load app
	app, err := Load(appFolderPath.String())
	assert.Error(t, err)
	assert.Empty(t, app)
	assert.ErrorContains(t, err, "main python file and sketch file missing from app")
}
