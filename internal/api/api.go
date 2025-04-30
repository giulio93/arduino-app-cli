package api

import (
	"net/http"

	"github.com/arduino/arduino-app-cli/internal/api/handlers"

	dockerClient "github.com/docker/docker/client"
)

func NewHTTPRouter(dockerClient *dockerClient.Client) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/app/start", handlers.HandleAppStart(dockerClient))
	mux.Handle("POST /v1/app/stop", handlers.HandleAppStop(dockerClient))
	mux.Handle("GET /v1/app/list", handlers.HandleAppList(dockerClient))

	return mux
}
