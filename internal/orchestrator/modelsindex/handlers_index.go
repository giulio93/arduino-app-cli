// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package modelsindex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"slices"
	"time"

	composetmpl "github.com/compose-spec/compose-go/v2/template"
	"github.com/goccy/go-yaml"
	"github.com/moby/moby/client"
	"go.bug.st/f"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-app-cli/internal/dockerhelper"
	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

type HandlerActions struct {
	Download []string
	Delete   []string
	Check    []string
	Info     []string
}
type Action string

const (
	ActionDownload Action = "download"
	ActionDelete   Action = "delete"
	ActionCheck    Action = "check"
	ActionInfo     Action = "info"
)

func (a HandlerActions) validate(id string) error {
	for _, name := range []Action{ActionDownload, ActionDelete, ActionCheck} {
		if len(a.command(name)) == 0 {
			return fmt.Errorf("handler %q: missing required action %q", id, name)
		}
	}
	return nil
}

func (a HandlerActions) command(name Action) []string {
	switch name {
	case ActionDownload:
		return a.Download
	case ActionDelete:
		return a.Delete
	case ActionCheck:
		return a.Check
	case ActionInfo:
		return a.Info
	default:
		return nil
	}
}

func (h *HandlersIndex) runAction(ctx context.Context, cli client.APIClient, handler ModelHandler, action Action, vars map[string]string, lineParser func(string)) error {

	command := handler.Actions.command(action)
	if len(command) == 0 {
		return fmt.Errorf("handler %q: %w: %q", handler.ID, ErrNoAction, action)
	}
	env := maps.Clone(vars)
	maps.Insert(env, maps.All(h.configEnv))

	var stdout io.Writer = io.Discard
	if lineParser != nil {
		stdout = f.NewCallbackWriter(lineParser)
	}

	return dockerhelper.Run(ctx, cli, dockerhelper.RunOptions{
		Image:  ResolveVars(handler.Image, env),
		Cmd:    command,
		Binds:  ResolveVarsSlice(handler.Volumes, env),
		Env:    env,
		Stdout: stdout,
		Stderr: f.NewCallbackWriter(func(line string) {
			slog.Debug("handler stderr", "handler", handler.ID, "action", action, "line", line)
		}),
	})

}

type ModelHandler struct {
	ID      string
	Image   string
	Volumes []string
	Actions HandlerActions
}

const handlersFileName = "models-handlers.yaml"

func loadHandlers(dir *paths.Path, modelsDir *paths.Path, cfg config.Configuration, plat platform.Platform) (*HandlersIndex, error) {
	// TODO : we should add a method on config to return env variables
	configEnv := map[string]string{
		"DOCKER_REGISTRY_BASE": cfg.DockerRegistryBase(),
		"BOARD_NAME":           plat.BoardName,
		"MODELS_PATH":          modelsDir.String(),
	}

	handlersFile := dir.Join(handlersFileName)
	if handlersFile.NotExist() {
		return nil, nil
	}

	content, err := handlersFile.ReadFile()
	if err != nil {
		return nil, err
	}

	var raw rawHandlersList
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("models-handlers.yaml: %w", err)
	}

	var listing *ListingConfig
	if raw.Listing.Image != "" {
		listing = &ListingConfig{
			Image:   raw.Listing.Image,
			Volumes: raw.Listing.Volumes,
			Command: raw.Listing.Command,
		}
	}

	handlers := make(map[string]ModelHandler, len(raw.Handlers))
	for _, handlerMap := range raw.Handlers {
		for id, entry := range handlerMap {
			if id == "" {
				return nil, fmt.Errorf("models-handlers.yaml: handler has empty id")
			}
			if entry.Image == "" {
				return nil, fmt.Errorf("models-handlers.yaml: handler %q missing required field \"image\"", id)
			}
			var actions HandlerActions
			for _, actionMap := range entry.Actions {
				for name, actionEntry := range actionMap {
					switch name {
					case "download":
						actions.Download = actionEntry.Command
					case "delete":
						actions.Delete = actionEntry.Command
					case "check":
						actions.Check = actionEntry.Command
					case "info":
						actions.Info = actionEntry.Command
					}
				}
			}
			if err := actions.validate(id); err != nil {
				return nil, fmt.Errorf("models-handlers.yaml: %w", err)
			}
			if len(entry.Volumes) == 0 {
				return nil, fmt.Errorf("models-handlers.yaml: handler %q missing required field \"volumes\"", id)
			}
			handlers[id] = ModelHandler{
				ID:      id,
				Image:   entry.Image,
				Volumes: entry.Volumes,
				Actions: actions,
			}
		}
	}

	return &HandlersIndex{handlers: handlers, listing: listing, configEnv: configEnv}, nil
}

// WriteHandlers writes into dir a models-handlers.yaml holding only the handlers named,
// as the assets file states them. freeze answers their ${VAR} before the file lands.
func WriteHandlers(assetDir *paths.Path, dir *paths.Path, ids []string, freeze func([]byte) ([]byte, error)) error {
	content, err := assetDir.Join(handlersFileName).ReadFile()
	if err != nil {
		return err
	}

	var raw rawHandlersList
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return fmt.Errorf("%s: %w", handlersFileName, err)
	}

	raw.Handlers = slices.DeleteFunc(raw.Handlers, func(entry map[string]rawHandlerEntry) bool {
		return !slices.ContainsFunc(ids, func(id string) bool { _, declares := entry[id]; return declares })
	})
	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	if data, err = freeze(data); err != nil {
		return fmt.Errorf("%s: %w", handlersFileName, err)
	}
	return dir.Join(handlersFileName).WriteFile(data)
}

// resolveVars substitutes compose-style ${VAR} and ${VAR:-default} placeholders
// in raw using the provided vars map. Unknown variables are left unchanged.
func ResolveVars(raw string, vars map[string]string) string {
	result, err := composetmpl.Substitute(raw, func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	})
	if err != nil {
		slog.Warn("cannot resolve template variables", "raw", raw, "err", err)
		return raw
	}
	return result
}

// ResolveVarsSlice applies ResolveVars to each string in raws and returns a new slice with the results.
func ResolveVarsSlice(raws []string, vars map[string]string) []string {
	return f.Map(raws, func(v string) string {
		return ResolveVars(v, vars)
	})
}

type ListingConfig struct {
	Image   string
	Volumes []string
	Command []string
}

type HandlersIndex struct {
	handlers  map[string]ModelHandler
	listing   *ListingConfig
	configEnv map[string]string
}

func (h *HandlersIndex) GetHandlerByID(id string) (ModelHandler, bool) {
	handler, ok := h.handlers[id]
	return handler, ok
}

func (h *HandlersIndex) GetListingConfig() *ListingConfig {
	return h.listing
}

// mibToBytes converts a size_mb (MiB, rounded to 0.01) to bytes.
func mibToBytes(v float64) uint64 {
	const mib = 1 << 20
	return uint64(math.Round(v * mib))
}

// runListAction runs the listing container, which writes the index into the models dir.
// What it prints is not read.
func runListAction(ctx context.Context, cli client.APIClient, listing *ListingConfig, configEnv map[string]string) error {
	slog.Debug("running list action", "image", listing.Image)

	var stderr bytes.Buffer
	start := time.Now()
	err := dockerhelper.Run(ctx, cli, dockerhelper.RunOptions{
		Image:  ResolveVars(listing.Image, configEnv),
		Cmd:    listing.Command,
		Binds:  ResolveVarsSlice(listing.Volumes, configEnv),
		Env:    configEnv,
		Stdout: io.Discard,
		Stderr: &stderr,
	})
	slog.Debug("list action finished", "duration_s", time.Since(start).Seconds())
	if err != nil {
		return fmt.Errorf("list action: %w: %s", err, stderr.String())
	}
	return nil
}

type MessageType string

const (
	UnknownType  MessageType = ""
	ProgressType MessageType = "progress"
	InfoType     MessageType = "info"
	ErrorType    MessageType = "error"
	DoneType     MessageType = "done"
)

type StreamMessage struct {
	err      string
	data     string
	progress *Progress
	done     string
	model    *DownloadedModel
}

type DownloadedModel struct {
	ID   string
	Size uint64
}

type Progress struct {
	Name     string
	Total    int64
	Current  int64
	Progress float32
}

func (p *StreamMessage) IsData() bool           { return p.data != "" }
func (p *StreamMessage) IsError() bool          { return p.err != "" }
func (p *StreamMessage) IsProgress() bool       { return p.progress != nil }
func (p *StreamMessage) IsDone() bool           { return p.done != "" }
func (p *StreamMessage) GetData() string        { return p.data }
func (p *StreamMessage) GetError() string       { return p.err }
func (p *StreamMessage) GetProgress() *Progress { return p.progress }
func (p *StreamMessage) GetDone() string        { return p.done }

// GetModel is nil until the handler names what it wrote, and stays nil for a handler too
// old to report it.
func (p *StreamMessage) GetModel() *DownloadedModel { return p.model }
func (p *StreamMessage) GetType() MessageType {
	if p.IsData() {
		return InfoType
	}
	if p.IsProgress() {
		return ProgressType
	}
	if p.IsError() {
		return ErrorType
	}
	if p.IsDone() {
		return DoneType
	}
	return UnknownType
}

// The events a download handler reports. StreamMessage keeps its fields unexported, so a
// message always carries exactly one kind of payload.
func NewInfoMessage(description string, model *DownloadedModel) StreamMessage {
	return StreamMessage{data: description, model: model}
}

func NewProgressMessage(p Progress) StreamMessage {
	return StreamMessage{progress: &p}
}

func NewErrorMessage(description string) StreamMessage {
	return StreamMessage{err: description}
}

func NewDoneMessage(description string) StreamMessage {
	return StreamMessage{done: description}
}

type HandlerEvent struct {
	Event       string   `json:"event"` // start, update, complete, info, stat, error
	Description string   `json:"description"`
	Current     int64    `json:"current"`     // update: bytes so far
	Total       int64    `json:"total"`       // update: bytes expected
	SizeBytes   *int64   `json:"size_bytes"`  // stat (HF); -1 or null means unknown
	SizeMB      *float64 `json:"size_mb"`     // stat, info, complete: MiB, 2 decimals
	ModelID     string   `json:"model_id"`    // info: the id of what was downloaded
	Downloading *bool    `json:"downloading"` // check: nil when the event does not say
	Status      string   `json:"status"`      // check: installed, not_installed or in_progress
	ErrorCode   string   `json:"code"`        // error: install_in_progress when the model's lock is held
	Artifacts   []string `json:"artifacts"`
}

// The status a check action reports.
const (
	CheckInstalled    = "installed"
	CheckNotInstalled = "not_installed"
	CheckInProgress   = "in_progress"
)

// codeInstallInProgress is the error a download or delete reports when another one holds
// the model's lock in its container.
const codeInstallInProgress = "install_in_progress"

func parseHandlerEvent(line string) (e HandlerEvent, ok bool) {
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		return HandlerEvent{}, false
	}
	return e, true
}

func (e HandlerEvent) Size() uint64 {
	if e.SizeBytes != nil && *e.SizeBytes > 0 {
		return uint64(*e.SizeBytes)
	}
	if e.SizeMB != nil && *e.SizeMB > 0 {
		return mibToBytes(*e.SizeMB)
	}
	return 0
}
func (e HandlerEvent) IsError() bool { return e.Event == "error" }

func (h *HandlersIndex) GetDockerImages() []string {
	if h == nil {
		slog.Warn("handlers index is nil, cannot get model handler images")
		return []string{}
	}

	images := make(map[string]struct{})
	for _, handler := range h.handlers {
		image := ResolveVars(handler.Image, h.configEnv)
		images[image] = struct{}{}
	}

	if h.listing != nil && h.listing.Image != "" {
		image := ResolveVars(h.listing.Image, h.configEnv)
		images[image] = struct{}{}
	}

	return slices.Collect(maps.Keys(images))
}

type rawActionEntry struct {
	Command []string `yaml:"command"`
}

type rawHandlerEntry struct {
	Description string                      `yaml:"description"`
	Image       string                      `yaml:"image"`
	Volumes     []string                    `yaml:"volumes"`
	Actions     []map[string]rawActionEntry `yaml:"actions"`
}

type rawListingEntry struct {
	Image   string   `yaml:"image"`
	Volumes []string `yaml:"volumes"`
	Command []string `yaml:"command"`
}

type rawHandlersList struct {
	Listing  rawListingEntry              `yaml:"listing"`
	Handlers []map[string]rawHandlerEntry `yaml:"handlers"`
}

func (h *HandlersIndex) deleteInternalModel(ctx context.Context, cli client.APIClient, model AIModel, handler ModelHandler, plat platform.Platform, lineParser func(line string)) error {
	return h.runAction(ctx, cli, handler, ActionDelete, model.Deployment.VariablesForPlatform(plat.BoardName), lineParser)
}

func (h *HandlersIndex) downloadModel(ctx context.Context, cli client.APIClient, model AIModel, handler ModelHandler, plat platform.Platform, lineParser func(line string)) error {
	model.Deployment.VariablesForPlatform(plat.BoardName)
	return h.runAction(ctx, cli, handler, ActionDownload, model.Deployment.VariablesForPlatform(plat.BoardName), lineParser)
}

// checkLineStatus reads one line of a check action: its status, empty for a line that
// does not say.
func checkLineStatus(line string) string {
	e, ok := parseHandlerEvent(line)
	if !ok {
		return ""
	}
	return e.Status
}

// isBusyLine reports whether a download or delete line says another one holds the model.
func isBusyLine(line string) bool {
	e, ok := parseHandlerEvent(line)
	return ok && e.IsError() && e.ErrorCode == codeInstallInProgress
}
