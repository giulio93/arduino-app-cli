package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/arduino/arduino-app-cli/pkg/parser"

	"github.com/arduino/go-paths-helper"
	"github.com/docker/docker/api/types/container"
	dockerClient "github.com/docker/docker/client"
	"go.bug.st/f"
	"gopkg.in/yaml.v3"
)

func ProvisionApp(ctx context.Context, pythonImage string, docker *dockerClient.Client, app parser.App) {
	pullBasePythonContainer(ctx, pythonImage)
	resp, err := docker.ContainerCreate(ctx, &container.Config{
		Image:      pythonImage,
		User:       getCurrentUser(),
		Entrypoint: []string{"/run.sh", "provision"},
	}, &container.HostConfig{
		Binds:      []string{app.FullPath.String() + ":/app"},
		AutoRemove: true,
	}, nil, nil, app.Name)
	if err != nil {
		log.Panic(err)
	}

	fmt.Println("\nLaunching the base Python image to get the modules/bricks details...")

	waitCh, errCh := docker.ContainerWait(ctx, resp.ID, container.WaitConditionNextExit)
	if err := docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Panic(err)
	}
	fmt.Println("Provisioning container started with ID:", resp.ID)

	select {
	case result := <-waitCh:
		if result.Error != nil {
			log.Panic("Provisioning container wait error:", result.Error.Message)
		}
		fmt.Println("Provisioning container exited with status code:", result.StatusCode)
	case err := <-errCh:
		log.Panic("Error waiting for container:", err)
	}

	generateMainComposeFile(ctx, app, pythonImage)
}

func pullBasePythonContainer(ctx context.Context, pythonImage string) {
	process, err := paths.NewProcess(nil, "docker", "pull", pythonImage)
	if err != nil {
		log.Panic(err)
	}
	process.RedirectStdoutTo(os.Stdout)
	process.RedirectStderrTo(os.Stderr)
	err = process.RunWithinContext(ctx)
	if err != nil {
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

func generateMainComposeFile(ctx context.Context, app parser.App, pythonImage string) {
	provisioningStateDir := getProvisioningStateDir(app)

	var composeFiles paths.PathList
	for _, dep := range app.Descriptor.ModuleDependencies {
		composeFilePath := provisioningStateDir.Join("compose", dep.Name, "module_compose.yaml")
		if composeFilePath.Exist() {
			composeFiles.Add(composeFilePath)
			fmt.Printf("- Using module: %s\n", dep.Name)
		} else {
			fmt.Printf("- Using module: %s (not found)\n", dep.Name)
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

	fmt.Printf("\nCompose file for the App '\033[0;35m%s\033[0m' created, launching...\n", app.Name)

	// docker compose -f app-compose.yml config --services
	process, err := paths.NewProcess(nil, "docker", "compose", "-f", mainComposeFile.String(), "config", "--services")
	if err != nil {
		log.Panic(err)
	}
	stdout, stderr, err := process.RunAndCaptureOutput(ctx)
	if err != nil {
		log.Panic(err, " stderr:"+string(stderr))
	}
	if len(stderr) > 0 {
		fmt.Println("stderr:", string(stderr))
	}
	services := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	services = f.Filter(services, f.NotEquals("main"))

	ports := make([]string, len(app.Descriptor.Ports))
	for i, p := range app.Descriptor.Ports {
		ports[i] = fmt.Sprintf("%d:%d", p, p)
	}

	mainAppCompose.Services = &mainService{
		Main: service{
			Image:      pythonImage,
			Volumes:    []string{app.FullPath.String() + ":/app"},
			Ports:      ports,
			Devices:    getDevices(),
			Entrypoint: "/run.sh",
			DependsOn:  services,
			User:       getCurrentUser(),
		},
	}
	writeMainCompose()
}
