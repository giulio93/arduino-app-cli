package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/user"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-app-cli/pkg/parser"
)

func StartApp(ctx context.Context, app parser.App) {
	provisioningStateDir := getProvisioningStateDir(app)
	mainCompose := provisioningStateDir.Join("app-compose.yaml")
	process, err := paths.NewProcess(nil, "docker", "compose", "-f", mainCompose.String(), "up", "-d", "--remove-orphans")
	if err != nil {
		log.Panic(err)
	}
	process.RedirectStdoutTo(os.Stdout)
	process.RedirectStderrTo(os.Stderr)
	err = process.RunWithinContext(ctx)
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("App '\033[0;35m%s\033[0m' started: Docker Compose running in detached mode.\n", app.Name)
}

func StopApp(ctx context.Context, app parser.App) {
	provisioningStateDir := getProvisioningStateDir(app)
	mainCompose := provisioningStateDir.Join("app-compose.yaml")
	process, err := paths.NewProcess(nil, "docker", "compose", "-f", mainCompose.String(), "stop")
	if err != nil {
		log.Panic(err)
	}
	process.RedirectStdoutTo(os.Stdout)
	process.RedirectStderrTo(os.Stderr)
	err = process.RunWithinContext(ctx)
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("Containers for the App '\033[0;35m%s\033[0m' stopped and removed\n", app.Name)
}

func AppLogs(ctx context.Context, app parser.App) {
	provisioningStateDir := getProvisioningStateDir(app)
	mainCompose := provisioningStateDir.Join("app-compose.yaml")
	process, err := paths.NewProcess(nil, "docker", "compose", "-f", mainCompose.String(), "logs", "main", "-f")
	if err != nil {
		log.Panic(err)
	}
	process.RedirectStdoutTo(os.Stdout)
	process.RedirectStderrTo(os.Stderr)
	err = process.RunWithinContext(ctx)
	if err != nil {
		log.Println(err)
	}
}

type ListAppResult struct {
	Stdout []byte
	Stderr []byte
}

func ListApps(ctx context.Context) (ListAppResult, error) {
	process, err := paths.NewProcess(nil, "docker", "compose", "ls", "-a")
	if err != nil {
		log.Panic(err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	// stream output
	process.RedirectStdoutTo(stdout)
	process.RedirectStderrTo(stderr)

	err = process.RunWithinContext(ctx)
	if err != nil {
		return ListAppResult{}, err
	}
	return ListAppResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}, nil
}

func getCurrentUser() string {
	// Map user to avoid permission issues.
	// MacOS and Windows uses a VM so we don't need to map the user.
	user, err := user.Current()
	if err != nil {
		panic(err)
	}
	return user.Uid + ":" + user.Gid
}

func getDevices() []string {
	deviceList, err := paths.New("/dev").ReadDir()
	if err != nil {
		panic(err)
	}
	deviceList.FilterPrefix("video")
	return deviceList.AsStrings()
}
