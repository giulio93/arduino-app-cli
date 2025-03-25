package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/docker/docker/api/types/container"
	dockerClient "github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

const pythonImage = "arduino-pythonenv"

func main() {
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		panic(err)
	}
	defer docker.Close()

	rootCmd := &cobra.Command{
		Use:   "app",
		Short: "A CLI to manage the Python app",
	}

	rootCmd.AddCommand(
		&cobra.Command{
			Use:   "stop",
			Short: "Stop the Python app",
			Run: func(cmd *cobra.Command, args []string) {
				stopHandler(docker, args[0])
			},
		},
		&cobra.Command{
			Use:   "start",
			Short: "Start the Python app",
			Run: func(cmd *cobra.Command, args []string) {
				startHandler(docker, args[0])
			},
		},
		&cobra.Command{
			Use:   "logs",
			Short: "Show the logs of the Python app",
			Run: func(cmd *cobra.Command, args []string) {
				logsHandler(docker, args[0])
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List all running Python apps",
			Run: func(cmd *cobra.Command, args []string) {
				listHandler(docker)
			},
		},
	)

	if err := rootCmd.Execute(); err != nil {
		log.Panic(err)
	}
}

func startHandler(docker *dockerClient.Client, appPath string) {
	ctx := context.Background()

	app := filepath.Base(appPath)

	absAppPath, err := filepath.Abs(appPath)
	if err != nil {
		log.Panic(err)
	}

	// Map user to avoid permission issues.
	// MacOS and Windows uses a VM so we don't need to map the user.
	var userMapping string
	if runtime.GOOS == "linux" {
		user, err := user.Current()
		if err != nil {
			log.Panic("cannot get linux user: %w", err)
		}
		userMapping = user.Uid + ":" + user.Gid
	}

	name := appToContainerName(app)
	resp, err := docker.ContainerCreate(ctx, &container.Config{
		Image: pythonImage,
		User:  userMapping,
	}, &container.HostConfig{
		Binds:      []string{absAppPath + ":/app"},
		AutoRemove: true,
	}, nil, nil, name)
	if err != nil {
		log.Panic(err)
	}

	if err := docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Panic(err)
	}

	fmt.Println("Container started with ID:", resp.ID)
}

func stopHandler(docker *dockerClient.Client, appPath string) {
	ctx := context.Background()

	app := filepath.Base(appPath)

	name := appToContainerName(app)
	id, err := findContainer(ctx, docker, name)
	if err != nil {
		log.Panic(err)
	}

	if err := docker.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		log.Panic(err)
	}

	fmt.Println("Container stopped and removed")
}

func logsHandler(docker *dockerClient.Client, appPath string) {
	ctx := context.Background()

	app := filepath.Base(appPath)

	name := appToContainerName(app)
	id, err := findContainer(ctx, docker, name)
	if err != nil {
		log.Panic(err)
	}

	out, err := docker.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		log.Panic(err)
	}
	defer out.Close()

	if _, err := io.Copy(os.Stdout, out); err != nil {
		log.Panic(err)
	}
}

func listHandler(docker *dockerClient.Client) {
	ctx := context.Background()

	resp, err := docker.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		log.Panic(err)
	}

	for _, container := range resp {
		if strings.HasPrefix(container.Names[0], "/arduino_") && strings.HasSuffix(container.Names[0], "_python") {
			fmt.Println(containerToAppName(container.Names[0]))
		}
	}
}

func appToContainerName(app string) string {
	app = strings.Trim(app, "/\n ")
	app = strings.ReplaceAll(app, "/", "_")
	return fmt.Sprintf("arduino_%s_python", app)
}

func containerToAppName(name string) string {
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimPrefix(name, "arduino_")
	name = strings.TrimSuffix(name, "_python")
	return name
}

func findContainer(ctx context.Context, docker *dockerClient.Client, containerName string) (string, error) {
	resp, err := docker.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return "", err
	}

	idx := slices.IndexFunc(resp, func(container container.Summary) bool {
		return container.Names[0] == "/"+containerName // Container name is prefixed with a slash
	})
	if idx == -1 {
		return "", fmt.Errorf("container not found")
	}

	return resp[idx].ID, nil
}
