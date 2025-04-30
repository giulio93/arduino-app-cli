package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/arduino/arduino-app-cli/internal/orchestrator"
	"github.com/arduino/arduino-app-cli/pkg/parser"
	"github.com/arduino/arduino-app-cli/pkg/render"

	dockerClient "github.com/docker/docker/client"
)

type appStartRequest struct {
	AppPath string `json:"app_path"`
}

func HandleAppStart(dockerClient *dockerClient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req appStartRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			render.EncodeResponse(w, http.StatusBadRequest, "unable to parse the request")
		}

		if req.AppPath == "" {
			render.EncodeResponse(w, http.StatusPreconditionFailed, "path cannot be empty")
		}

		app, err := parser.Load(req.AppPath)
		if err != nil {
			slog.Error("Unable to parse the app.yaml", slog.String("error", err.Error()), slog.String("path", req.AppPath))
			render.EncodeResponse(w, http.StatusInternalServerError, "unable to find the app")
			return
		}
		orchestrator.StartApp(r.Context(), app)
		render.EncodeResponse(w, http.StatusOK, "app started")
	}
}

type appStopRequest struct {
	AppPath string `json:"app_path"`
}

func HandleAppStop(dockerClient *dockerClient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req appStopRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			render.EncodeResponse(w, http.StatusBadRequest, "unable to parse the request")
		}

		if req.AppPath == "" {
			render.EncodeResponse(w, http.StatusPreconditionFailed, "path cannot be empty")
		}

		app, err := parser.Load(req.AppPath)
		if err != nil {
			slog.Error("Unable to parse the app.yaml", slog.String("error", err.Error()), slog.String("path", req.AppPath))
			render.EncodeResponse(w, http.StatusInternalServerError, "unable to find the app")
			return
		}
		orchestrator.StopApp(r.Context(), app)
		render.EncodeResponse(w, http.StatusOK, "app stopped")
	}
}

func HandleAppList(dockerClient *dockerClient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := orchestrator.ListApps(r.Context())
		if err != nil {
			slog.Error("Unable to parse the app.yaml", slog.String("error", err.Error()))
			render.EncodeResponse(w, http.StatusInternalServerError, "unable to find the app")
			return
		}

		render.EncodeResponse(w, http.StatusOK, string(res.Stdout))
	}
}
