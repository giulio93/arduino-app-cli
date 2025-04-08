package parser

import (
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"
)

func TestAppParser(t *testing.T) {
	appPath := paths.New("testdata", "app.yaml")
	app, err := ParseDescriptorFile(appPath)
	require.NoError(t, err)
	golden, err := appPath.ReadFile()
	require.NoError(t, err)
	res, err := app.AsYaml()
	require.NoError(t, err)
	require.Equal(t, string(golden), string(res))
}
