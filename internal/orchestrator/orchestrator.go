package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"slices"

	"github.com/arduino/arduino-cli/commands"
	rpc "github.com/arduino/arduino-cli/rpc/cc/arduino/cli/commands/v1"
	"github.com/arduino/go-paths-helper"
	"github.com/sirupsen/logrus"

	"github.com/arduino/arduino-app-cli/pkg/parser"
)

func StartApp(ctx context.Context, app parser.App) {
	// Build and upload the sketch
	if app.MainSketchFile != nil {
		if err := compileUploadSketch(ctx, app.MainSketchFile.String()); err != nil {
			log.Panic(err)
		}
	}

	// Run Python app
	if app.MainPythonFile != nil {
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
}

func StopApp(ctx context.Context, app parser.App) {
	// Stop sketch
	if app.MainSketchFile != nil {
		// Flash empty sketch to stop the microcontroller.
		// TODO: check that the app sketch is running before attempting to stop it.
		if err := compileUploadSketch(ctx, getEmptySketch()); err != nil {
			log.Panic(err)
		}
	}
	// Stop python app
	if app.MainPythonFile != nil {
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

func compileUploadSketch(ctx context.Context, path string) error {
	logrus.SetLevel(logrus.ErrorLevel)
	srv := commands.NewArduinoCoreServer()

	var inst *rpc.Instance
	if resp, err := srv.Create(ctx, &rpc.CreateRequest{}); err != nil {
		return err
	} else {
		inst = resp.GetInstance()
	}

	defer func() {
		_, _ = srv.Destroy(ctx, &rpc.DestroyRequest{Instance: inst})
	}()

	if err := srv.Init(
		&rpc.InitRequest{Instance: inst},
		commands.InitStreamResponseToCallbackFunction(ctx, nil),
	); err != nil {
		return err
	}

	resp, err := srv.BoardList(ctx, &rpc.BoardListRequest{
		Instance:                      inst,
		Timeout:                       0,
		Fqbn:                          "",
		SkipCloudApiForBoardDetection: false,
	})
	if err != nil {
		return err
	}

	idx := slices.IndexFunc(resp.Ports, func(p *rpc.DetectedPort) bool {
		return len(p.MatchingBoards) > 0
	})
	if idx == -1 {
		return fmt.Errorf("no board detected")
	}

	name := resp.Ports[idx].MatchingBoards[0].Name
	fqbn := resp.Ports[idx].MatchingBoards[0].Fqbn
	port := resp.Ports[idx].Port
	fmt.Println("\nAuto selected board:", name, "fqbn:", fqbn, "port:", port.Address)

	// build the sketch
	server, _ := commands.CompilerServerToStreams(ctx, os.Stdout, os.Stderr, func(msg *rpc.TaskProgress) {})

	// TODO: add build cache
	// TODO: maybe handle resultCB.GetDiagnostics()
	err = srv.Compile(&rpc.CompileRequest{
		Instance:   inst,
		Fqbn:       fqbn,
		SketchPath: path,
	}, server)
	if err != nil {
		return err
	}

	stream, _ := commands.UploadToServerStreams(ctx, os.Stdout, os.Stderr)
	err = srv.Upload(&rpc.UploadRequest{
		Instance:   inst,
		Fqbn:       fqbn,
		SketchPath: path,
		Port:       port,
	}, stream)
	if err != nil {
		return err
	}

	return nil
}

func getEmptySketch() string {
	const emptySketch = `void setup() {}
void loop() {}
`
	dir := filepath.Join(os.TempDir(), "empty_sketch")
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Panic(err)
	}
	ino := filepath.Join(dir, "empty_sketch.ino")
	err := os.WriteFile(ino, []byte(emptySketch), 0600)
	if err != nil {
		panic(err)
	}
	return ino
}
