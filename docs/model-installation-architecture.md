# Model Installation Architecture

This document explains how `arduino-app-cli` downloads and installs AI models from various sources: Qualcomm AI Hub, Hugging Face, and Edge Impulse.

## Overview

The `InstallModelByID` orchestrator dispatches model downloads to source-specific handlers based on a `deployment` block in the model manifest (from `models-list.yaml`). The handlers are:

- **`ai-hub-handler`** — Qualcomm AI Hub models
- **`hf-handler`** — Hugging Face models  
- **`ei-handler`** — Edge Impulse models

Each handler is implemented as an in-process Go client in `internal/api/{aihub,huggingface,edgeimpulse}`. This document focuses on the design decisions behind the AI Hub and Hugging Face handlers.

---

## Qualcomm AI Hub Handler

### Why We Don't Use the `qai_hub_models` CLI

The Qualcomm AI Hub models are available via a Python CLI tool (`qai-hub-models-cli`), which the Python downloader in `app-bricks-py` uses. However, `arduino-app-cli` implements the download logic directly in Go rather than shelling out to the Python CLI.

**Rationale:**
1. **No subprocess overhead** — Calling Python CLI requires spawning a subprocess, which adds latency
2. **Language-native implementation** — Go is the primary language for this project
3. **Simpler dependency management** — No need to require Python environment on the deployment device
4. **Direct API integration** — We implement the same logic as the CLI, making it transparent and testable

### URL Resolution: Mirroring `fetch.py`

The Go implementation (`internal/api/aihub/aihub_client.go`) replicates the exact URL resolution logic from the Qualcomm CLI's `fetch.py`:

```go
// Go: AssetURLs generates candidate URLs with fallback
urls := []string{
    // Chipset-specific first
    "{base}/{name}-{runtime}-{quantization}-{chipset}.zip",
    // Fall back to generic
    "{base}/{name}-{runtime}-{quantization}.zip",
}

// ResolveAssetURL tries each candidate via HEAD request
for _, url := range urls {
    if resp.StatusCode == 200 { return url }
}
```

This mirrors the Python CLI's `get_asset_url()` function (lines 105–123 of `fetch.py` in `quic/ai-hub-models`):

```python
# Python (Qualcomm CLI)
# Not all runtimes produce chipset-specific assets. 
# Try the chipset URL first, then fall back to the generic URL.
if chipset is not None:
    url, _ = _asset_url(model_id, runtime, precision, version, chipset)
    if _head(url) == 200:
        return url

url, _ = _asset_url(model_id, runtime, precision, version)
if _head(url) == 200:
    return url

raise FileNotFoundError(...)
```

### Why HEAD Requests Instead of a Manifest?

Both the Python CLI and our Go implementation use HTTP HEAD requests to check asset availability. This is a workaround for AI Hub's lack of a file listing API.

As noted in the Qualcomm source code (TODO #18374):
> "Read available assets from a manifest instead of making HEAD requests."

**Future:** When Qualcomm AI Hub exposes a manifest API, both implementations will be updated to use it instead of HEAD requests. The Go implementation will need minimal changes — just replacing the `ResolveAssetURL` logic.

---

## Hugging Face Handler

### Two Input Formats for Flexibility

Like the Python downloader, the Go handler supports two deployment variable formats for specifying Hugging Face models:

**Format 1 — Compact (model_key):**
```yaml
deployment:
  variables:
    model_key: "llamacpp:unsloth/gemma-4-E4B-it-GGUF:Q4_0:BF16"
```

Parsed as: `<type>:<repo_id>:<quantization>[:<mmproj_quantization>]`

**Format 2 — Explicit:**
```yaml
deployment:
  variables:
    model_repo_id: "unsloth/gemma-4-E4B-it-GGUF"
    model_name: "gemma-4-E4B-it-Q4_0.gguf"
    model_mmproj_name: "mmproj-BF16.gguf"  # optional
```

Both formats are transformed into fnmatch-style glob patterns and passed to the Hugging Face API to download matching files.

### REST API vs. Official Library

The Go implementation uses the Hugging Face REST API directly (`https://huggingface.co/api/models`) rather than the official Python library (`huggingface_hub`). Both approaches are equivalent:

- **Python:** Uses `huggingface_hub.snapshot_download()` for file enumeration and download
- **Go:** Uses REST API directly for file enumeration and HTTP download

This choice allows `arduino-app-cli` to remain language-native without adding Python dependencies.

---

## ZIP Stream Extraction

Both AI Hub and Hugging Face handlers download ZIP archives. The Go implementation extracts them using `internal/orchestrator/modelsindex/custommodel/zip.go`, which:

1. **Buffers the stream to a temporary file** (same filesystem as destination to avoid cross-device moves)
2. **Extracts with `archive/zip`** (Go's standard library)
3. **Validates path traversal attempts** to prevent directory escape exploits
4. **Returns the list of extracted files** for verification

This is simpler than the Python implementation's streaming extraction with `stream_unzip`, but trades some memory efficiency for code simplicity. For typical model sizes (100s of MB to GBs), buffering to a temp file is acceptable.

---

## Deployment Block Format

Models declare their download source and platform-specific parameters via a `deployment` block in `models-list.yaml`:

```yaml
models:
  - id: "melotts_en"
    name: "MeloTTS English"
    runner: "brick"
    deployment:
      handler: "ai-hub-handler"
      platforms:
        - ventunoq:
            variables:
              model_name: "melotts_en"
              model_type: "voice_ai"
              quantization: "mixed_with_float"
              chipset: "qualcomm-qcs8275"
              version: "0.51.0"
              model_directory: "melotts_en-voice_ai-mixed_with_float-qualcomm_qcs8275"
              models_repository: "/opt/models"
```

The handler validates that all required variables are present (`mustVar`) and uses optional variables as fallbacks (`optionalVar`).

---

## Authentication & Credentials

**None of the three download paths require user-supplied credentials:**

- **AI Hub:** Assets are on a public S3 bucket (no authentication)
- **Hugging Face:** Public repos work without a token (deployment block may carry `hf_token` for gated repos)
- **Edge Impulse:** Uses the parameterized `/deployment/download` endpoint with platform-specific credentials embedded in the model metadata (no x-api-key required from the user)

This keeps the model installation flow simple and secure.

---

## Error Handling

Each handler:
- Returns meaningful errors if required deployment variables are missing
- Propagates HTTP errors (connectivity, 404s, etc.) for debugging
- Validates ZIP integrity during extraction
- Reports the final install location for the caller to record in persistent state

