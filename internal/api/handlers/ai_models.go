// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"go.bug.st/f"

	"github.com/arduino/arduino-app-cli/internal/api/edgeimpulse"
	"github.com/arduino/arduino-app-cli/internal/api/models"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/appstore"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/modelsindex"
	"github.com/arduino/arduino-app-cli/internal/render"
)

type InstallEIModelRequest struct {
	ImpulseID *int `json:"impulse_id" description:"Edge Impulse impulse ID" example:"1" required:"true"`
}

// ModelInstaller installs, downloads and deletes models. The daemon passes
// orchestrator.NewModelInstaller; a nil one (boardless mode) answers 501 needs_board.
type ModelInstaller interface {
	Install(ctx context.Context, id string, publish func(modelsindex.StreamMessage)) (modelsindex.AIModel, error)
	Download(ctx context.Context, modelURL, mmprojURL string, publish func(modelsindex.StreamMessage)) (modelsindex.AIModel, error)
	InstallEI(ctx context.Context, eiClient *edgeimpulse.EIClient, projectID, impulseID int) (modelsindex.AIModel, error)
	Delete(ctx context.Context, id string, force bool) error
}

// NeedsBoardCode is the error code of a route that needs a board.
const NeedsBoardCode = "needs_board"

// NeedsBoard answers a route that needs a board where there is none.
func NeedsBoard() http.HandlerFunc { return needsBoard }

func needsBoard(w http.ResponseWriter, r *http.Request) {
	render.EncodeResponse(w, http.StatusNotImplemented, models.ErrorResponse{
		Code:    NeedsBoardCode,
		Details: "this needs a board: " + r.Pattern,
	})
}

func HandleModelsList(modelsIndex *modelsindex.ModelsIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var brickFilter []string
		if bricks := strings.TrimSpace(r.URL.Query().Get("bricks")); bricks != "" {
			brickFilter = strings.Split(bricks, ",")
		}
		list, err := appstore.AIModelsList(r.Context(), appstore.AIModelsListRequest{
			FilterByBrickID: brickFilter,
		}, modelsIndex)
		if err != nil {
			// Without the listing, every model would report the status its declaration
			// carries - not-installed - so a partial answer would be a wrong one.
			slog.Error("cannot get models info", "err", err)
			render.EncodeResponse(w, http.StatusInternalServerError, models.ErrorResponse{
				Details: "cannot determine which models are installed: " + err.Error(),
			})
			return
		}
		render.EncodeResponse(w, http.StatusOK, models.AIModelsListResult{
			Models: f.Map(list, models.NewAIModelItem),
		})
	}
}

// modelIDFromPath reads the model id a path names, so the plain id exists only below
// this line.
func modelIDFromPath(r *http.Request) (string, error) {
	return models.DecodeModelID(r.PathValue("modelID"))
}

func HandlerModelByID(modelsIndex *modelsindex.ModelsIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := modelIDFromPath(r)
		if err != nil {
			render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: err.Error()})
			return
		}
		model, found, err := appstore.AIModelDetails(r.Context(), modelsIndex, id)
		if err != nil {
			render.EncodeResponse(w, http.StatusInternalServerError, models.ErrorResponse{Details: err.Error()})
			return
		}
		if !found {
			details := fmt.Sprintf("models with id %q not found", id)
			render.EncodeResponse(w, http.StatusNotFound, models.ErrorResponse{Details: details})
			return
		}
		render.EncodeResponse(w, http.StatusOK, models.NewAIModelItem(model))
	}
}

func HandlerDeleteModelByID(installer ModelInstaller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if installer == nil {
			needsBoard(w, r)
			return
		}
		id, err := modelIDFromPath(r)
		if err != nil {
			render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: err.Error()})
			return
		}
		forceRaw := r.URL.Query().Get("force")
		force, err := strconv.ParseBool(forceRaw)
		if err != nil {
			force = false
		}

		err = installer.Delete(r.Context(), id, force)
		if err != nil {
			switch {
			case errors.Is(err, appstore.ErrNotFound):
				render.EncodeResponse(w, http.StatusNotFound, models.ErrorResponse{Details: err.Error()})
			case errors.Is(err, appstore.ErrConflict):
				render.EncodeResponse(w, http.StatusConflict, models.ErrorResponse{Details: err.Error()})
			case errors.Is(err, appstore.ErrCannotRemoveModel):
				render.EncodeResponse(w, http.StatusConflict, models.ErrorResponse{Details: err.Error()})
			default:
				render.EncodeResponse(w, http.StatusInternalServerError, models.ErrorResponse{Details: err.Error()})
			}
			return
		}

		render.EncodeResponse(w, http.StatusNoContent, nil)
	}
}

func HandleInstallEIModel(cfg config.Configuration, installer ModelInstaller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if installer == nil {
			needsBoard(w, r)
			return
		}
		projectID, err := strconv.Atoi(r.PathValue("projectID"))
		if err != nil {
			render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: "invalid projectID"})
			return
		}
		prjApiKey := r.Header.Get("x-api-key")
		if prjApiKey == "" {
			render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: "x-api-key header must be set"})
			return
		}

		var req InstallEIModelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			slog.Error("unable to decode download EI model request", slog.String("error", err.Error()))
			render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: "unable to decode download EI model request"})
			return
		}

		if err := req.Validate(); err != nil {
			render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: err.Error()})
			return
		}

		eiClient, err := edgeimpulse.NewEIClient(prjApiKey, *cfg.EdgeImpulseAPIURL)
		if err != nil {
			slog.Error("unable to create Edge Impulse client", slog.String("error", err.Error()))
			render.EncodeResponse(w, http.StatusInternalServerError, models.ErrorResponse{Details: "unable to create Edge Impulse client"})
			return
		}

		eiModel, err := installer.InstallEI(r.Context(), eiClient, projectID, *req.ImpulseID)
		if err != nil {
			switch {
			case errors.Is(err, edgeimpulse.ErrUnauthorized):
				slog.Error("unauthorized access to Edge Impulse model", slog.String("error", err.Error()))
				render.EncodeResponse(w, http.StatusUnauthorized, models.ErrorResponse{Details: "unauthorized access to Edge Impulse model"})
				return
			case errors.Is(err, appstore.ErrIncompleteImpulse):
				slog.Error("incomplete impulse for Edge Impulse model", slog.String("error", err.Error()))
				render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: "incomplete impulse for Edge Impulse model"})
				return
			case errors.Is(err, edgeimpulse.ErrForbidden):
				slog.Error("forbidden access to Edge Impulse model", slog.String("error", err.Error()))
				render.EncodeResponse(w, http.StatusForbidden, models.ErrorResponse{Details: "forbidden access to Edge Impulse model"})
				return
			case errors.Is(err, appstore.ErrInsufficientStorage):
				slog.Error("insufficient storage to install Edge Impulse model", slog.String("error", err.Error()))
				render.EncodeResponse(w, http.StatusInsufficientStorage, models.ErrorResponse{Details: "insufficient storage to install Edge Impulse model"})
				return
			default:
				slog.Error("unable to install Edge Impulse model", slog.String("error", err.Error()))
				render.EncodeResponse(w, http.StatusInternalServerError, models.ErrorResponse{Details: "unable to install Edge Impulse model: " + err.Error()})
				return
			}
		}

		// FIXME: read the installed model using the modelindex.getModelByID
		render.EncodeResponse(w, http.StatusOK, models.NewAIModelItem(eiModel))
	}
}

func (r InstallEIModelRequest) Validate() error {
	if r.ImpulseID == nil || *r.ImpulseID <= 0 {
		return fmt.Errorf("impulse_id must be an integer greater than 0")
	}
	return nil
}

type sseProgress struct {
	Name     string  `json:"name"`
	Total    int64   `json:"total"`
	Current  int64   `json:"current"`
	Progress float32 `json:"progress"`
}

type SSELog struct {
	Message string `json:"message"`
}

// HandleInstallModel installs a model from the internal model list. HandleDownloadModel
// downloads a model that no entry declares.
func HandleInstallModel(installer ModelInstaller, modelsIndex *modelsindex.ModelsIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if installer == nil {
			needsBoard(w, r)
			return
		}
		id, err := modelIDFromPath(r)
		if err != nil {
			render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: err.Error()})
			return
		}

		// A 404 has to be a status, so this one question is asked before the stream opens.
		if !modelsIndex.IsKnown(id) {
			details := fmt.Sprintf("no model with id %q is declared", id)
			render.EncodeResponse(w, http.StatusNotFound, models.ErrorResponse{Details: details})
			return
		}

		sseStream, err := render.NewSSEStream(r.Context(), w)
		if err != nil {
			slog.Error("unable to create SSE stream", slog.String("error", err.Error()))
			render.EncodeResponse(w, http.StatusInternalServerError, models.ErrorResponse{Details: "unable to create SSE stream"})
			return
		}
		defer sseStream.Close()

		stream := &downloadStream{sse: sseStream}
		installed, err := installer.Install(r.Context(), id, stream.publish)
		if err != nil {
			stream.sendError(err)
			return
		}
		sseStream.Send(render.SSEEvent{Type: render.DoneEvent, Data: models.NewAIModelItem(installed)})
	}
}

type DownloadModelRequest struct {
	ModelURL  string `json:"model_url" example:"https://huggingface.co/unsloth/SmolLM2-135M-Instruct-GGUF/resolve/main/SmolLM2-135M-Instruct-Q4_K_M.gguf" required:"true"`
	MmprojURL string `json:"mmproj_url" example:"https://huggingface.co/unsloth/SmolLM2-135M-Instruct-GGUF/resolve/main/mmproj-F16.gguf"`
}

// HandleDownloadModel downloads a model that no models-list.yaml entry declares. The id is
// not an input: the downloader makes it from the file that arrives, and reports it.
func HandleDownloadModel(installer ModelInstaller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if installer == nil {
			needsBoard(w, r)
			return
		}
		var req DownloadModelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: "unable to decode download model request"})
			return
		}
		modelURL := strings.TrimSpace(req.ModelURL)
		if modelURL == "" {
			render.EncodeResponse(w, http.StatusBadRequest, models.ErrorResponse{Details: "model_url must be set"})
			return
		}

		sseStream, err := render.NewSSEStream(r.Context(), w)
		if err != nil {
			slog.Error("unable to create SSE stream", slog.String("error", err.Error()))
			render.EncodeResponse(w, http.StatusInternalServerError, models.ErrorResponse{Details: "unable to create SSE stream"})
			return
		}
		defer sseStream.Close()

		stream := &downloadStream{sse: sseStream}
		installed, err := installer.Download(r.Context(), modelURL, strings.TrimSpace(req.MmprojURL), stream.publish)
		if err != nil {
			stream.sendError(err)
			return
		}
		sseStream.Send(render.SSEEvent{Type: render.DoneEvent, Data: models.NewAIModelItem(installed)})
	}
}

// sseSender is the part of render.SSEStream a download needs. An interface so a test can
// read the events back without an http.ResponseWriter and the stream's goroutine.
type sseSender interface {
	Send(event render.SSEEvent)
	SendError(event render.SSEErrorData)
}

// downloadStream sends a handler's download events as SSE. After the stream opens, a
// failure is an event, not an HTTP status.
type downloadStream struct {
	sse sseSender
}

func (d *downloadStream) publish(e modelsindex.StreamMessage) {
	switch e.GetType() {
	case modelsindex.InfoType:
		d.sse.Send(render.SSEEvent{Type: render.MessageEvent, Data: SSELog{Message: e.GetData()}})
	case modelsindex.ProgressType:
		p := e.GetProgress()
		var progress float32
		if p.Total > 0 {
			progress = float32(p.Current) / float32(p.Total) * 100
		}
		d.sse.Send(render.SSEEvent{Type: render.ProgressEvent, Data: sseProgress{
			Name: p.Name, Current: p.Current, Total: p.Total, Progress: progress,
		}})
	case modelsindex.ErrorType:
		if code := render.SSEErrCode(e.GetErrorCode()); code != "" {
			d.sse.SendError(render.SSEErrorData{Code: code, Message: e.GetError()})
		} else {
			d.sse.SendError(render.SSEErrorData{Code: render.InternalServiceErr, Message: e.GetError()})
		}
	case modelsindex.DoneType:
		d.sse.Send(render.SSEEvent{Type: render.MessageEvent, Data: SSELog{Message: e.GetDone()}})
	}
}

func (d *downloadStream) sendError(err error) {
	if errors.Is(err, modelsindex.ErrDownloadReported) {
		// The handler's own error event went out through publish.
		slog.Error("download reported an error", "err", err)
		return
	}
	if errors.Is(err, modelsindex.ErrInsufficientStorage) {
		d.sse.SendError(render.SSEErrorData{Code: "insufficient_storage", Message: err.Error()})
		return
	}
	d.sse.SendError(render.SSEErrorData{Code: render.InternalServiceErr, Message: err.Error()})
}
