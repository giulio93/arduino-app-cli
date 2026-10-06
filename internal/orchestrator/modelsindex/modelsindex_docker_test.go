// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package modelsindex

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"iter"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-app-cli/internal/orchestrator/config"
	"github.com/arduino/arduino-app-cli/internal/platform"
)

// fakeDockerClient intercepts ContainerCreate/Wait/Attach/Start.
// All other client.APIClient methods panic — they must not be called.
type fakeDockerClient struct {
	client.APIClient

	runFunc func(image string, cmd []string) (stdout string, exitCode int)
	// runFuncEnv, when set, is called instead of runFunc and also receives the
	// container's environment.
	runFuncEnv func(image string, cmd, env []string) (stdout string, exitCode int)

	mu        sync.Mutex
	idCounter int
	pending   map[string]*pendingContainer

	// onStop is called by ContainerStop, which dockerhelper.Run sends when its context ends:
	// it lets a fake container that blocks finish, as a real one does on SIGTERM.
	onStop func()
}

type pendingContainer struct {
	image      string
	cmd        []string
	env        []string
	attachConn net.Conn
	statusCh   chan container.WaitResponse
	errCh      chan error
}

func newFakeDockerClient(runFunc func(image string, cmd []string) (stdout string, exitCode int)) *fakeDockerClient {
	return &fakeDockerClient{
		runFunc: runFunc,
		pending: make(map[string]*pendingContainer),
	}
}

// fakeCommandCli is the docker command.Cli the callers that take one are given: they only
// read its client.
type fakeCommandCli struct {
	command.Cli
	cli client.APIClient
}

func (f fakeCommandCli) Client() client.APIClient { return f.cli }

func newFakeDockerClientWithEnv(runFunc func(image string, cmd, env []string) (stdout string, exitCode int)) *fakeDockerClient {
	return &fakeDockerClient{
		runFuncEnv: runFunc,
		pending:    make(map[string]*pendingContainer),
	}
}

func (f *fakeDockerClient) ContainerCreate(_ context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idCounter++
	id := fmt.Sprintf("fake-%d", f.idCounter)
	cfg := options.Config
	f.pending[id] = &pendingContainer{image: cfg.Image, cmd: cfg.Cmd, env: cfg.Env}
	return client.ContainerCreateResult{ID: id}, nil
}

func (f *fakeDockerClient) ContainerWait(_ context.Context, id string, _ client.ContainerWaitOptions) client.ContainerWaitResult {
	statusCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)
	f.mu.Lock()
	f.pending[id].statusCh = statusCh
	f.pending[id].errCh = errCh
	f.mu.Unlock()
	return client.ContainerWaitResult{Result: statusCh, Error: errCh}
}

func (f *fakeDockerClient) ContainerAttach(_ context.Context, id string, _ client.ContainerAttachOptions) (client.ContainerAttachResult, error) {
	clientConn, serverConn := net.Pipe()
	f.mu.Lock()
	f.pending[id].attachConn = serverConn
	f.mu.Unlock()
	return client.ContainerAttachResult{HijackedResponse: client.HijackedResponse{
		Conn:   clientConn,
		Reader: bufio.NewReader(clientConn),
	}}, nil
}

func (f *fakeDockerClient) ContainerStart(_ context.Context, id string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
	f.mu.Lock()
	p := f.pending[id]
	delete(f.pending, id)
	f.mu.Unlock()

	go func() {
		var stdout string
		var exitCode int
		if f.runFuncEnv != nil {
			stdout, exitCode = f.runFuncEnv(p.image, p.cmd, p.env)
		} else {
			stdout, exitCode = f.runFunc(p.image, p.cmd)
		}
		if stdout != "" {
			writeStdoutFrame(p.attachConn, stdout)
		}
		p.attachConn.Close()
		p.statusCh <- container.WaitResponse{StatusCode: int64(exitCode)}
	}()
	return client.ContainerStartResult{}, nil
}

func (f *fakeDockerClient) ContainerStop(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	if f.onStop != nil {
		f.onStop()
	}
	return client.ContainerStopResult{}, nil
}

func (f *fakeDockerClient) ContainerRemove(_ context.Context, _ string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, nil
}

func (f *fakeDockerClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return fakePullResponse{ReadCloser: io.NopCloser(strings.NewReader(""))}, nil
}

// fakePullResponse is what the client returns for a pull: a stream, plus the two ways
// of reading it the api offers.
type fakePullResponse struct {
	io.ReadCloser
}

func (fakePullResponse) JSONMessages(context.Context) iter.Seq2[jsonstream.Message, error] {
	return func(func(jsonstream.Message, error) bool) {}
}

func (fakePullResponse) Wait(context.Context) error { return nil }

func (f *fakeDockerClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{}, nil
}

// writeStdoutFrame multiplexes the payload the way the daemon does, which is what
// stdcopy.StdCopy unframes on the other side.
func writeStdoutFrame(w io.Writer, payload string) {
	header := make([]byte, 8)
	header[0] = byte(stdcopy.Stdout)
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload))) // nolint:gosec // a test payload is small
	_, _ = w.Write(header)
	_, _ = io.WriteString(w, payload)
}

// listModelsCmd is the listing container's command, as testdata/with-handlers declares it.
const listModelsCmd = "/app/list_models.sh"

// Index entries as the listing writes them, one {id: model} map each.
const (
	piperEntry        = `piper-tts-en: {name: Piper TTS, bricks: [{id: "arduino:tts"}], deployment: {handler: ai-hub-handler, pre-loaded: true}, status: installed, size_bytes: 48234496, origin: curated}`
	efficientNetEntry = `"ei:efficientnet-b4": {name: EfficientNet-B4, bricks: [{id: "arduino:image_classification"}], deployment: {handler: ei-handler, platforms: [{ventunoq: {variables: {model_name: efficientnet-b4.eim}}}]}, metadata: {model_size_mb: 89, source: edgeimpulse, runtime: edge-impulse-sdk}, status: not-installed, size_bytes: 93323264, origin: curated}`
	// downloadedEntry is the user model the downloads below write: its deployment is the
	// record's inputs, so the link it came from is its model_url.
	downloadedEntry = `"llamacpp:org/repo/m-Q4_0": {name: org/repo/m-Q4_0, handler: hf-handler, bricks: [{id: "arduino:llm"}], deployment: {handler: hf-handler, platforms: [{ventunoq: {variables: {models_repository: llamacpp, model_url: "llamacpp:org/repo:Q4_0"}}}]}, metadata: {source-model-url: "llamacpp:org/repo:Q4_0"}, status: installed, size_bytes: 1048576, folder: llamacpp/org/repo, origin: user}`
)

// writeIndex writes the index a listing would, holding entries.
func writeIndex(t *testing.T, modelsDir *paths.Path, entries ...string) {
	t.Helper()
	doc := "models: []\n"
	if len(entries) > 0 {
		doc = "models:\n  - " + strings.Join(entries, "\n  - ") + "\n"
	}
	require.NoError(t, modelsDir.Join(modelsIndexFileName).WriteFile([]byte(doc)))
}

// indexedClient is a docker client whose listing container writes entries into the index
// of modelsDir. Any other container is answered by other; listings counts the listing runs.
func indexedClient(t *testing.T, modelsDir *paths.Path, entries []string, other func(cmd []string) (string, int)) (*fakeDockerClient, *atomic.Int64) {
	var listings atomic.Int64
	cli := newFakeDockerClient(func(_ string, cmd []string) (string, int) {
		if len(cmd) > 0 && cmd[0] == listModelsCmd {
			listings.Add(1)
			writeIndex(t, modelsDir, entries...)
			return "", 0
		}
		if other == nil {
			t.Errorf("unexpected container %v", cmd)
			return "", 1
		}
		return other(cmd)
	})
	return cli, &listings
}

func loadTestIndex(t *testing.T, modelsDir *paths.Path, cli client.APIClient) *ModelsIndex {
	t.Helper()
	dir := paths.New("testdata/with-handlers")
	idx, err := Load(platform.Platform{BoardName: "ventunoq"}, dir, modelsDir, dir.Join("custom-models"), cli, config.Configuration{})
	require.NoError(t, err)
	return idx
}

func TestGetModelByID_WithDockerMock(t *testing.T) {
	t.Run("the custom modeldir volume is not resolved at load time", func(t *testing.T) {
		cli := newFakeDockerClient(func(image string, cmd []string) (string, int) { return "", 0 })
		idx := loadTestIndex(t, paths.New(t.TempDir()), cli)

		require.Equal(t, []string{"${MODELS_PATH}:/models"}, idx.Handlers.listing.Volumes)
		h, ok := idx.Handlers.GetHandlerByID("ai-hub-handler")
		require.True(t, ok)
		require.Equal(t, []string{"${MODELS_PATH:-/var/lib/arduino-app-cli/models}:/models"}, h.Volumes)
	})

	t.Run("state and size come from the index", func(t *testing.T) {
		modelsDir := paths.New(t.TempDir())
		cli, _ := indexedClient(t, modelsDir, []string{piperEntry, efficientNetEntry}, nil)
		idx := loadTestIndex(t, modelsDir, cli)

		piper, err := idx.NewLookup().ByID(t.Context(), "piper-tts-en")
		require.NoError(t, err)
		require.NotNil(t, piper)
		assert.Equal(t, InstalledStatus, piper.Status)
		assert.Equal(t, uint64(48234496), piper.SizeBytes)
		assert.True(t, piper.Preinstalled, "pre-loaded by its declaration")
		assert.Equal(t, CuratedOrigin, piper.Origin)

		ei, err := idx.NewLookup().ByID(t.Context(), "ei:efficientnet-b4")
		require.NoError(t, err)
		require.NotNil(t, ei)
		assert.Equal(t, NotInstalledStatus, ei.Status)
		assert.Equal(t, uint64(93323264), ei.SizeBytes)
		assert.False(t, ei.Preinstalled)
	})

	t.Run("listing fails with no index: an error rather than a declared status", func(t *testing.T) {
		cli := newFakeDockerClient(func(image string, cmd []string) (string, int) { return "", 1 })
		idx := loadTestIndex(t, paths.New(t.TempDir()), cli)

		_, err := idx.NewLookup().ByID(t.Context(), "ei:efficientnet-b4")
		require.Error(t, err)
		// Only the index lists undeclared models, so without it no id can be called absent.
		_, err = idx.NewLookup().ByID(t.Context(), "no-such-model-id")
		require.Error(t, err)
	})

	t.Run("a listing that writes nothing is an error", func(t *testing.T) {
		cli := newFakeDockerClient(func(image string, cmd []string) (string, int) { return "", 0 })
		idx := loadTestIndex(t, paths.New(t.TempDir()), cli)

		_, err := idx.NewLookup().All(t.Context())
		require.ErrorContains(t, err, modelsIndexFileName)
	})

	t.Run("an empty index is an empty catalog", func(t *testing.T) {
		modelsDir := paths.New(t.TempDir())
		writeIndex(t, modelsDir)
		idx := loadTestIndex(t, modelsDir, newFakeDockerClient(nil))

		_, err := idx.NewLookup().All(t.Context())
		require.ErrorIs(t, err, ErrEmptyCatalog)
	})

	t.Run("ei-model-990187-1 custom model: installed, found by the folder scan", func(t *testing.T) {
		// The index knows nothing of custom models: the folder scan finds it.
		modelsDir := paths.New(t.TempDir())
		writeIndex(t, modelsDir, piperEntry)
		idx := loadTestIndex(t, modelsDir, newFakeDockerClient(nil))

		model, err := idx.NewLookup().ByID(t.Context(), "ei-model-990187-1")
		require.NoError(t, err)
		require.NotNil(t, model)
		assert.Equal(t, InstalledStatus, model.Status)
	})
}

// TestIndexDescribesTheModel covers what the index carries beyond the status: a user
// model nothing declares is described by it alone.
func TestIndexDescribesTheModel(t *testing.T) {
	modelsDir := paths.New(t.TempDir())
	writeIndex(t, modelsDir, efficientNetEntry, downloadedEntry,
		`"ei:other": {name: Other, deployment: {handler: ei-handler}, status: downloading, origin: curated}`)
	idx := loadTestIndex(t, modelsDir, newFakeDockerClient(nil))

	user, err := idx.NewLookup().ByID(t.Context(), "llamacpp:org/repo/m-Q4_0")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, UserOrigin, user.Origin)
	assert.Equal(t, "hf-handler", user.Handler)
	assert.Equal(t, []BrickConfig{{ID: "arduino:llm"}}, user.Bricks)
	assert.Equal(t, map[string]string{"source-model-url": "llamacpp:org/repo:Q4_0"}, user.Metadata)
	assert.Equal(t, "llamacpp:org/repo:Q4_0", user.Deployment.VariablesForPlatform("ventunoq")["model_url"],
		"a delete of it runs with the link it was downloaded from")
	assert.Equal(t, modelsDir.Join("llamacpp", "org", "repo"), user.ModelFolderPath)
	assert.False(t, user.Preinstalled)

	ei, err := idx.NewLookup().ByID(t.Context(), "ei:efficientnet-b4")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"model_size_mb": "89", "source": "edgeimpulse", "runtime": "edge-impulse-sdk"}, ei.Metadata)

	inFlight, err := idx.NewLookup().ByID(t.Context(), "ei:other")
	require.NoError(t, err)
	assert.Equal(t, DownloadingStatus, inFlight.Status)
}

// TestModelForBrick covers the write path: the lookup answers on plain ids, and reports
// the model under its own id so the caller stores that.
func TestModelForBrick(t *testing.T) {
	modelsDir := paths.New(t.TempDir())
	writeIndex(t, modelsDir, efficientNetEntry)
	idx := loadTestIndex(t, modelsDir, newFakeDockerClient(nil))

	model, err := idx.NewLookup().ModelForBrick(t.Context(), "ei:efficientnet-b4", "arduino:image_classification")
	require.NoError(t, err)
	require.NotNil(t, model)
	assert.Equal(t, "ei:efficientnet-b4", model.ID)

	// A brick the model does not serve is not a lookup failure, it is simply no match.
	other, err := idx.NewLookup().ModelForBrick(t.Context(), "ei:efficientnet-b4", "arduino:tts")
	require.NoError(t, err)
	assert.Nil(t, other)
}

// TestLookupReadsTheIndex pins when a listing container runs: only when no index exists
// yet, and on Refresh. Everything else reads the file.
func TestLookupReadsTheIndex(t *testing.T) {
	t.Run("no index: the first lookup runs one listing, later ones read the file", func(t *testing.T) {
		modelsDir := paths.New(t.TempDir())
		cli, listings := indexedClient(t, modelsDir, []string{piperEntry, efficientNetEntry}, nil)
		idx := loadTestIndex(t, modelsDir, cli)

		lookup := idx.NewLookup()
		_, err := lookup.ByID(t.Context(), "ei:efficientnet-b4")
		require.NoError(t, err)
		_, err = lookup.ByBrick(t.Context(), "arduino:image_classification")
		require.NoError(t, err)
		_, err = idx.NewLookup().ByID(t.Context(), "piper-tts-en")
		require.NoError(t, err)

		assert.Equal(t, int64(1), listings.Load())
	})

	t.Run("an index written meanwhile is what the next lookup reads", func(t *testing.T) {
		modelsDir := paths.New(t.TempDir())
		writeIndex(t, modelsDir, efficientNetEntry)
		cli, listings := indexedClient(t, modelsDir, nil, nil)
		idx := loadTestIndex(t, modelsDir, cli)

		before, err := idx.NewLookup().ByID(t.Context(), "ei:efficientnet-b4")
		require.NoError(t, err)
		assert.Equal(t, NotInstalledStatus, before.Status)

		// What a handler does before its container exits.
		writeIndex(t, modelsDir, strings.Replace(efficientNetEntry, "status: not-installed", "status: installed", 1))
		after, err := idx.NewLookup().ByID(t.Context(), "ei:efficientnet-b4")
		require.NoError(t, err)
		assert.Equal(t, InstalledStatus, after.Status)
		assert.Zero(t, listings.Load())
	})

	t.Run("Refresh runs the listing again", func(t *testing.T) {
		modelsDir := paths.New(t.TempDir())
		writeIndex(t, modelsDir, efficientNetEntry)
		cli, listings := indexedClient(t, modelsDir, []string{piperEntry}, nil)
		idx := loadTestIndex(t, modelsDir, cli)

		models, err := idx.Refresh(t.Context())
		require.NoError(t, err)
		assert.Equal(t, int64(1), listings.Load())
		assert.Equal(t, "piper-tts-en", models[0].ID, "the answer is what the listing wrote")
	})

	t.Run("a failed listing is retried by the next Lookup", func(t *testing.T) {
		var failed atomic.Int64
		cli := newFakeDockerClient(func(_ string, cmd []string) (string, int) {
			if len(cmd) > 0 && cmd[0] == listModelsCmd {
				failed.Add(1)
			}
			return "", 1
		})
		idx := loadTestIndex(t, paths.New(t.TempDir()), cli)

		lookup := idx.NewLookup()
		_, err1 := lookup.ByID(t.Context(), "ei:efficientnet-b4")
		_, err2 := lookup.All(t.Context())
		require.Error(t, err1)
		require.Error(t, err2)
		assert.Equal(t, int64(1), failed.Load(), "one Lookup, one attempt")

		_, err := idx.NewLookup().ByID(t.Context(), "ei:efficientnet-b4")
		require.Error(t, err)
		assert.Equal(t, int64(2), failed.Load(), "a new Lookup retries")
	})

	t.Run("concurrent lookups with no index share one listing", func(t *testing.T) {
		modelsDir := paths.New(t.TempDir())
		var listings atomic.Int64
		release := make(chan struct{})
		cli := newFakeDockerClient(func(_ string, cmd []string) (string, int) {
			listings.Add(1)
			<-release
			writeIndex(t, modelsDir, piperEntry)
			return "", 0
		})
		idx := loadTestIndex(t, modelsDir, cli)

		const callers = 5
		var wg sync.WaitGroup
		errs := make(chan error, callers)
		for range callers {
			wg.Go(func() {
				_, err := idx.NewLookup().All(context.Background())
				errs <- err
			})
		}
		require.Eventually(t, func() bool { return listings.Load() == 1 }, time.Second, time.Millisecond)
		time.Sleep(50 * time.Millisecond) // the other callers join the run in flight
		close(release)
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		assert.Equal(t, int64(1), listings.Load())
	})
}

func TestRefreshEmptyCatalog(t *testing.T) {
	tmp := paths.New(t.TempDir())
	require.NoError(t, tmp.Join("models-list.yaml").WriteFile([]byte("models: []\n")))

	// No handlers file and no Docker client: the dry catalog is the whole answer.
	idx, err := Load(platform.Platform{}, tmp, tmp, nil, nil, config.Configuration{})
	require.NoError(t, err)

	models, err := idx.Refresh(t.Context())
	require.ErrorIs(t, err, ErrEmptyCatalog)
	assert.Nil(t, models)
}

// downloadDone is what the hf-handler prints once it has recorded the model.
const downloadDone = `{"event":"info","description":"Downloaded to: /models/org/repo","artifacts":["/models/org/repo/m-Q4_0.gguf"],"model_id":"llamacpp:org/repo/m-Q4_0","size_mb":1}` + "\n"

// TestDownloadByURL pins what reaches the container for an undeclared model: the
// hf-handler's script, the caller's URL, and models_repository fixed to llamacpp. The
// handler rewrites the index itself, so no listing runs after it.
func TestDownloadByURL(t *testing.T) {
	modelsDir := paths.New(t.TempDir())
	writeIndex(t, modelsDir, piperEntry)
	var gotCmd []string
	var gotEnv []string
	var listings atomic.Int64
	cli := newFakeDockerClientWithEnv(func(_ string, cmd, env []string) (string, int) {
		if len(cmd) > 0 && cmd[0] == listModelsCmd {
			listings.Add(1)
			return "", 0
		}
		if len(cmd) > 0 && strings.Contains(cmd[0], "hf_model_downloader.sh") {
			gotCmd, gotEnv = cmd, env
			writeIndex(t, modelsDir, piperEntry, downloadedEntry)
		}
		return downloadDone, 0
	})
	idx := loadTestIndex(t, modelsDir, cli)

	installed, err := idx.DownloadByURL(t.Context(), cli, "llamacpp:org/repo:Q4_0", "", platform.Platform{BoardName: "ventunoq"}, func(StreamMessage) {})
	require.NoError(t, err)

	require.NotEmpty(t, gotCmd, "the hf-handler download action must run")
	assert.Contains(t, gotEnv, "model_url=llamacpp:org/repo:Q4_0")
	assert.Contains(t, gotEnv, "models_repository=llamacpp")
	assert.NotContains(t, strings.Join(gotEnv, " "), "model_mmproj_url", "an empty mmproj url must not be passed")

	// The answer is the listed model, so the caller reports what a later list reports.
	assert.Equal(t, "llamacpp:org/repo/m-Q4_0", installed.ID)
	assert.Equal(t, InstalledStatus, installed.Status)
	assert.Equal(t, uint64(1024*1024), installed.SizeBytes)
	assert.Zero(t, listings.Load(), "the handler wrote the index")
}

// A handler whose index rewrite failed leaves the model unlisted: one listing run fixes it.
func TestDownloadByURLListsWhenTheHandlerCouldNot(t *testing.T) {
	modelsDir := paths.New(t.TempDir())
	writeIndex(t, modelsDir, piperEntry)
	cli, listings := indexedClient(t, modelsDir, []string{piperEntry, downloadedEntry}, func([]string) (string, int) {
		return downloadDone, 0
	})
	idx := loadTestIndex(t, modelsDir, cli)

	installed, err := idx.DownloadByURL(t.Context(), cli, "llamacpp:org/repo:Q4_0", "", platform.Platform{BoardName: "ventunoq"}, func(StreamMessage) {})
	require.NoError(t, err)
	assert.Equal(t, InstalledStatus, installed.Status)
	assert.Equal(t, int64(1), listings.Load())
}

// A repository already on disk is not transferred again: the handler reports the model it
// finds, with no "complete" event, and the route answers from the index.
func TestDownloadByURLReportsAnInstalledModel(t *testing.T) {
	modelsDir := paths.New(t.TempDir())
	writeIndex(t, modelsDir, downloadedEntry)
	cli := newFakeDockerClient(func(_ string, cmd []string) (string, int) {
		return `{"event":"info","description":"Model exists: org/repo (m-Q4_0.gguf)","artifacts":["/models/org/repo/m-Q4_0.gguf"],"model_id":"llamacpp:org/repo/m-Q4_0","size_mb":1}` + "\n", 0
	})
	idx := loadTestIndex(t, modelsDir, cli)

	var messages []string
	installed, err := idx.DownloadByURL(t.Context(), cli, "llamacpp:org/repo:Q4_0", "", platform.Platform{BoardName: "ventunoq"}, func(e StreamMessage) {
		if e.IsData() {
			messages = append(messages, e.GetData())
		}
	})
	require.NoError(t, err)

	assert.Equal(t, "llamacpp:org/repo/m-Q4_0", installed.ID)
	assert.Equal(t, uint64(1024*1024), installed.SizeBytes, "the size is the one on disk, not a transfer total")
	assert.Equal(t, []string{"Model exists: org/repo (m-Q4_0.gguf)"}, messages)
}

// A release's catalog can declare a model the image's catalog does not: once downloaded,
// the declaration describes it.
func TestInstallOfAModelTheIndexDoesNotList(t *testing.T) {
	modelsDir := paths.New(t.TempDir())
	cli, listings := indexedClient(t, modelsDir, []string{piperEntry}, func([]string) (string, int) {
		return `{"event":"info","description":"Downloaded","model_id":"ei:efficientnet-b4","size_mb":2}` + "\n", 0
	})
	writeIndex(t, modelsDir, piperEntry)
	idx := loadTestIndex(t, modelsDir, cli)

	installed, err := idx.Install(t.Context(), fakeCommandCli{cli: cli}, "ei:efficientnet-b4", platform.Platform{BoardName: "ventunoq"}, func(StreamMessage) {})
	require.NoError(t, err)
	assert.Equal(t, "ei:efficientnet-b4", installed.ID)
	assert.Equal(t, InstalledStatus, installed.Status)
	assert.Equal(t, uint64(2*1024*1024), installed.SizeBytes)
	assert.Equal(t, int64(1), listings.Load(), "one listing tried before falling back to the declaration")
}

// A model installed by its declaration reaches no container. The install route answers it
// without calling Download at all, so this guards the other callers.
func TestDownloadRefusesAModelWithNothingToDownload(t *testing.T) {
	var started int
	cli := newFakeDockerClient(func(_ string, _ []string) (string, int) {
		started++
		return "", 0
	})
	idx := loadTestIndex(t, paths.New(t.TempDir()), cli)

	// A nil docker client: a model that needs no download must not read it.
	installed, err := idx.Install(t.Context(), nil, "piper-tts-en", platform.Platform{BoardName: "ventunoq"}, func(StreamMessage) {})

	require.NoError(t, err)
	assert.Equal(t, InstalledStatus, installed.Status, "a pre-loaded model is installed already")
	assert.Zero(t, started, "a pre-loaded model must not start the downloader")

	// The guard stays on the runner, for a caller that reaches it with such a model.
	preLoaded, ok := idx.known("piper-tts-en")
	require.True(t, ok)
	_, err = idx.runDownload(t.Context(), cli, *preLoaded, platform.Platform{BoardName: "ventunoq"}, func(StreamMessage) {})
	require.ErrorIs(t, err, ErrNoHandler)
}

// busyLine is what a download or delete prints when another one holds the model's lock in
// its container.
const busyLine = `{"event":"error","code":"install_in_progress","description":"Another operation is in progress on model: org/repo"}` + "\n"

// TestBusyModel: the lock lives in the containers, and the handler says when it is held.
func TestBusyModel(t *testing.T) {
	plat := platform.Platform{BoardName: "ventunoq"}

	t.Run("a download finding the lock held is ErrInstallInProgress, not a reported error", func(t *testing.T) {
		modelsDir := paths.New(t.TempDir())
		writeIndex(t, modelsDir, piperEntry)
		cli := newFakeDockerClient(func(_ string, _ []string) (string, int) { return busyLine, 75 })
		idx := loadTestIndex(t, modelsDir, cli)

		var published []StreamMessage
		_, err := idx.DownloadByURL(t.Context(), cli, "llamacpp:org/repo:Q4_0", "", plat, func(e StreamMessage) { published = append(published, e) })
		require.ErrorIs(t, err, ErrInstallInProgress)
		assert.NotErrorIs(t, err, ErrDownloadReported)
		assert.Empty(t, published, "the caller reports it once, with its own code")
	})

	t.Run("a delete finding the lock held is ErrInstallInProgress", func(t *testing.T) {
		modelsDir := paths.New(t.TempDir())
		writeIndex(t, modelsDir, downloadedEntry)
		cli := newFakeDockerClient(func(_ string, _ []string) (string, int) { return busyLine, 75 })
		idx := loadTestIndex(t, modelsDir, cli)

		model, err := idx.NewLookup().ByID(t.Context(), "llamacpp:org/repo/m-Q4_0")
		require.NoError(t, err)
		err = idx.Delete(t.Context(), fakeCommandCli{cli: cli}, plat, *model)
		require.ErrorIs(t, err, ErrInstallInProgress)
	})

	t.Run("a delete runs no listing: the handler rewrites the index", func(t *testing.T) {
		modelsDir := paths.New(t.TempDir())
		writeIndex(t, modelsDir, downloadedEntry)
		cli, listings := indexedClient(t, modelsDir, nil, func([]string) (string, int) {
			return `{"event":"info","description":"Model removed"}` + "\n", 0
		})
		idx := loadTestIndex(t, modelsDir, cli)

		model, err := idx.NewLookup().ByID(t.Context(), "llamacpp:org/repo/m-Q4_0")
		require.NoError(t, err)
		require.NoError(t, idx.Delete(t.Context(), fakeCommandCli{cli: cli}, plat, *model))
		assert.Zero(t, listings.Load())
	})
}

// TestDownloadCancelled: a caller that goes away mid-download stops the container, which
// cleans up and reports "interrupted"; the result is the cancellation, not a container
// error. A download that fails on its own stays a container error. Neither runs a listing:
// the index never listed the model as there.
func TestDownloadCancelled(t *testing.T) {
	const url = "llamacpp:org/repo:Q4_0"
	const interrupted = `{"event":"error","description":"Download interrupted by signal; partial files removed"}`
	plat := platform.Platform{BoardName: "ventunoq"}

	newIndex := func(t *testing.T, download func() (string, int)) (*ModelsIndex, *fakeDockerClient, *atomic.Int64) {
		t.Helper()
		modelsDir := paths.New(t.TempDir())
		writeIndex(t, modelsDir, piperEntry)
		cli, listings := indexedClient(t, modelsDir, []string{piperEntry}, func([]string) (string, int) { return download() })
		return loadTestIndex(t, modelsDir, cli), cli, listings
	}

	t.Run("canceled: the cancellation is the error", func(t *testing.T) {
		started := make(chan struct{})
		stopped := make(chan struct{})
		var stopOnce sync.Once
		idx, cli, listings := newIndex(t, func() (string, int) {
			close(started)
			<-stopped // a real download runs until the stop signal
			return interrupted + "\n", 130
		})
		cli.onStop = func() { stopOnce.Do(func() { close(stopped) }) }

		ctx, cancel := context.WithCancel(t.Context())
		errCh := make(chan error, 1)
		go func() {
			_, err := idx.DownloadByURL(ctx, cli, url, "", plat, func(StreamMessage) {})
			errCh <- err
		}()
		<-started
		cancel()

		err := <-errCh
		require.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, ErrDownloadReported, "the container's interrupted event follows the cancel, it is not the cause")
		assert.Zero(t, listings.Load())
	})

	t.Run("failed on its own: a container error", func(t *testing.T) {
		idx, cli, listings := newIndex(t, func() (string, int) {
			return `{"event":"error","description":"Download failed: disk full"}` + "\n", 1
		})

		_, err := idx.DownloadByURL(t.Context(), cli, url, "", plat, func(StreamMessage) {})
		require.ErrorIs(t, err, ErrDownloadReported)
		assert.Zero(t, listings.Load())
	})
}
