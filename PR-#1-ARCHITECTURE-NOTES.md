# PR #1: Model Installation Architecture — Key Design Decisions

## Summary

This PR implements in-process model downloaders for Qualcomm AI Hub, Hugging Face, and Edge Impulse. Rather than shelling out to the Qualcomm `qai_hub_models` CLI, we implement the same download logic directly in Go.

## Why No Subprocess to `qai_hub_models` CLI?

The Python `app-bricks-py` downloader uses the Qualcomm CLI tool `qai-hub-models-cli` for AI Hub downloads. However, `arduino-app-cli` implements this logic natively in Go (`internal/api/aihub/aihub_client.go`) for several reasons:

### Benefits of Native Implementation

1. **No subprocess overhead** — Eliminates latency and complexity of spawning a Python process
2. **Language-native** — Go is the primary language; keeps the CLI cohesive and self-contained
3. **Simpler dependencies** — No need to require Python runtime on deployment devices
4. **Transparent & testable** — The download logic is explicit and can be unit-tested

## URL Resolution: We Mirror `fetch.py`

The Go implementation replicates the exact URL resolution strategy from Qualcomm's official CLI (`quic/ai-hub-models` repository):

**Qualcomm's `fetch.py` (lines 105–123):**
```python
# Not all runtimes produce chipset-specific assets. Try the chipset URL
# first, then fall back to the generic (non-chipset) URL.

if chipset is not None:
    url, _ = _asset_url(model_id, runtime, precision, version, chipset)
    if _head(url) == 200:
        return url

url, _ = _asset_url(model_id, runtime, precision, version)
if _head(url) == 200:
    return url

raise FileNotFoundError(...)
```

**Our Go implementation (`aihub_client.go`):**
```go
// AssetURLs builds candidate URLs with fallback
urls := c.AssetURLs(spec)  // chipset-specific, then generic

// ResolveAssetURL finds the first one that exists
for _, u := range urls {
    if headRequest(u).StatusCode == 200 {
        return u
    }
}
```

### Why HEAD Requests?

Qualcomm AI Hub doesn't expose a file listing API, so both the official CLI and our implementation use HTTP HEAD requests to check asset availability. This is a pragmatic workaround.

**Future:** The Qualcomm team has a TODO (issue #18374) to expose a manifest API. When that happens, both implementations will switch from HEAD requests to querying the manifest. Our Go code will need only minimal changes to `ResolveAssetURL`.

## Hugging Face Handler

The Hugging Face handler supports the same two input formats as the Python implementation:

1. **Compact format:** `model_key="llamacpp:repo_id:quantization[:mmproj_quant]"`
2. **Explicit format:** `model_repo_id` + `model_name` + optional `model_mmproj_name`

Both are converted to fnmatch-style patterns and passed to the HF API, eliminating the need for the `huggingface_hub` Python library.

## Edge Impulse Handler

The Edge Impulse handler uses the parameterized `/deployment/download` endpoint with platform-specific credentials (no user-provided API key required).

---

## Testing & Validation

All three handlers are fully tested in `internal/orchestrator/models_install_test.go`:
- Unit tests for variable resolution
- Integration tests with mocked HTTP servers
- ZIP extraction validation (including path traversal security tests)

---

## Documentation

For detailed architecture, design rationale, and future migration paths, see `docs/model-installation-architecture.md`.
