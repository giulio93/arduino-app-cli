package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/user"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/docker/docker/api/types/container"
	dockerClient "github.com/docker/docker/client"
	"github.com/spf13/cobra"
	"go.bug.st/cleanup"
	"go.bug.st/f"
	"gopkg.in/yaml.v3"

	"github.com/arduino/arduino-app-cli/pkg/parser"
)

const pythonImage = "arduino-python-base:latest"

func main() {
	docker, err := dockerClient.NewClientWithOpts(
		dockerClient.FromEnv,
		dockerClient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		panic(err)
	}
	defer docker.Close()

	var parsedApp parser.App

	rootCmd := &cobra.Command{
		Use:   "app <APP_PATH>",
		Short: "A CLI to manage the Python app",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				_ = cmd.Help()
				os.Exit(1)
			}
			app, err := parser.Load(args[0])
			if err != nil {
				log.Panic(err)
			}
			parsedApp = app
		},
	}

	rootCmd.AddCommand(
		&cobra.Command{
			Use:   "stop",
			Short: "Stop the Python app",
			Run: func(cmd *cobra.Command, args []string) {
				stopHandler(cmd.Context(), parsedApp)
			},
		},
		&cobra.Command{
			Use:   "start",
			Short: "Start the Python app",
			Run: func(cmd *cobra.Command, args []string) {
				provisionHandler(cmd.Context(), docker, parsedApp)
				startHandler(cmd.Context(), parsedApp)
			},
		},
		&cobra.Command{
			Use:   "logs",
			Short: "Show the logs of the Python app",
			Run: func(cmd *cobra.Command, args []string) {
				logsHandler(cmd.Context(), parsedApp)
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List all running Python apps",
			Run: func(cmd *cobra.Command, args []string) {
				listHandler(cmd.Context(), parsedApp)
			},
		},
		&cobra.Command{
			Use:   "provision",
			Short: "Provision the Python app",
			Run: func(cmd *cobra.Command, args []string) {
				provisionHandler(cmd.Context(), docker, parsedApp)
			},
		},
	)

	ctx := context.Background()
	ctx, cancel := cleanup.InterruptableContext(ctx)
	defer cancel()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Panic(err)
	}
}

func getProvisioningStateDir(app parser.App) *paths.Path {
	cacheDir := app.FullPath.Join(".cache")
	if err := cacheDir.MkdirAll(); err != nil {
		panic(err)
	}
	return cacheDir
}

func provisionHandler(ctx context.Context, docker *dockerClient.Client, app parser.App) {
	pwd, _ := os.Getwd()
	resp, err := docker.ContainerCreate(ctx, &container.Config{
		Image:      pythonImage,
		User:       getCurrentUser(),
		Entrypoint: []string{"/run.sh", "provision"},
	}, &container.HostConfig{
		Binds: []string{
			app.FullPath.String() + ":/app",
			pwd + "/scripts/provision.py:/provision.py",
			pwd + "/scripts/run.sh:/run.sh",
		},
		AutoRemove: true,
	}, nil, nil, app.Name)
	if err != nil {
		log.Panic(err)
	}

	waitCh, errCh := docker.ContainerWait(ctx, resp.ID, container.WaitConditionNextExit)
	if err := docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Panic(err)
	}
	fmt.Println("Container started with ID:", resp.ID)

	select {
	case result := <-waitCh:
		if result.Error != nil {
			log.Panic("Container wait error:", result.Error.Message)
		}
		fmt.Println("Container exited with status code:", result.StatusCode)
	case err := <-errCh:
		log.Panic("Error waiting for container:", err)
	}

	generateMainComposeFile(ctx, app)
}

func generateMainComposeFile(ctx context.Context, app parser.App) {
	provisioningStateDir := getProvisioningStateDir(app)

	var composeFiles paths.PathList
	for _, dep := range app.Descriptor.ModuleDependencies {
		composeFilePath := provisioningStateDir.Join("compose", dep, "module_compose.yaml")
		if composeFilePath.Exist() {
			composeFiles.Add(composeFilePath)
		}
	}

	// Create a single docker-mainCompose that includes all the required services
	mainComposeFile := provisioningStateDir.Join("app-compose.yaml")

	type service struct {
		Image      string   `yaml:"image"`
		DependsOn  []string `yaml:"depends_on,omitempty"`
		Volumes    []string `yaml:"volumes"`
		Devices    []string `yaml:"devices"`
		Ports      []string `yaml:"ports"`
		User       string   `yaml:"user"`
		Entrypoint string   `yaml:"entrypoint"`
	}
	type mainService struct {
		Main service `yaml:"main"`
	}
	var mainAppCompose struct {
		Name     string       `yaml:"name"`
		Include  []string     `yaml:"include,omitempty"`
		Services *mainService `yaml:"services,omitempty"`
	}
	writeMainCompose := func() {
		data, _ := yaml.Marshal(mainAppCompose)
		if err := mainComposeFile.WriteFile(data); err != nil {
			log.Panic(err)
		}
	}

	// Merge compose
	mainAppCompose.Include = composeFiles.AsStrings()
	mainAppCompose.Name = app.Name
	writeMainCompose()

	// docker compose -f app-compose.yml config --services
	process, err := paths.NewProcess(nil, "docker", "compose", "-f", mainComposeFile.String(), "config", "--services")
	if err != nil {
		log.Panic(err)
	}
	stdout, stderr, err := process.RunAndCaptureOutput(ctx)
	if err != nil {
		log.Panic(err)
	}
	if len(stderr) > 0 {
		fmt.Println("stderr:", string(stderr))
	}
	pwd, _ := os.Getwd()
	services := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	services = f.Filter(services, f.NotEquals("main"))

	ports := make([]string, len(app.Descriptor.Ports))
	for i, p := range app.Descriptor.Ports {
		ports[i] = fmt.Sprintf("%d:%d", p, p)
	}

	mainAppCompose.Services = &mainService{
		Main: service{
			Image: pythonImage, // TODO: when we will handle versioning change this
			Volumes: []string{
				app.FullPath.String() + ":/app",
				pwd + "/scripts/run.sh:/run.sh",
			},
			Ports:      ports,
			Devices:    getDevices(),
			Entrypoint: "/run.sh",
			DependsOn:  services,
			User:       getCurrentUser(),
		},
	}
	writeMainCompose()
}

func startHandler(ctx context.Context, app parser.App) {
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

	fmt.Println("Docker Compose project started in detached mode.")
}

func stopHandler(ctx context.Context, app parser.App) {
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

	fmt.Println("Container stopped and removed")
}

// TODO: for now we show only logs for the main python container.
// In the future we should also add logs for other services too.
func logsHandler(ctx context.Context, app parser.App) {
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

// TODO: show arduino app in execution
func listHandler(ctx context.Context, app parser.App) {
	provisioningStateDir := getProvisioningStateDir(app)
	mainCompose := provisioningStateDir.Join("app-compose.yaml")
	process, err := paths.NewProcess(nil, "docker", "compose", "-f", mainCompose.String(), "ls")
	if err != nil {
		log.Panic(err)
	}
	// stream output
	process.RedirectStdoutTo(os.Stdout)
	process.RedirectStderrTo(os.Stderr)

	err = process.RunWithinContext(ctx)
	if err != nil {
		log.Panic(err)
	}
}

func getDevices() []string {
	deviceList, err := paths.New("/dev").ReadDir()
	if err != nil {
		panic(err)
	}
	deviceList.FilterPrefix("video")
	return deviceList.AsStrings()
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
