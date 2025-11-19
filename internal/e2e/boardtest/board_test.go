package boardtest

import (
	"fmt"
	"runtime"
	"testing"
)

const daemonHost = "127.0.0.1:8800"

// const sseAppEventsPath = "http://127.0.0.1:8800/v1/apps/events"
// const sseAppIDEventsPath = "http://127.0.0.1:8800/v1/apps/%s/events"

var arch = runtime.GOARCH

func TestBlinkBoard(t *testing.T) {

	tagAppCli := fetchDebPackageLatest(t, "build/stable", "arduino-app-cli")
	fetchDebPackageLatest(t, "build/stable", "arduino-router")
	majorTag := genMajorTag(t, tagAppCli)

	fmt.Printf("Updating from stable version %s to unstable version %s \n", tagAppCli, majorTag)
	fmt.Printf("Building local deb version %s \n", majorTag)
	buildDebVersion(t, "build", majorTag, arch)

}
