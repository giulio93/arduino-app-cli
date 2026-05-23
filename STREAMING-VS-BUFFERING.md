# Streaming vs. Buffering: Why Python Uses `stream-unzip`

## The Problem: Large Model Files

AI Hub models can be **hundreds of MB to several GB** in size. Downloading and extracting these files requires careful memory management, especially on resource-constrained devices like the UNO Q.

The Python implementation offers **two strategies**:

### Strategy 1: Streaming Extraction (DEFAULT)
```python
streaming: When ``True`` (default), uses ``stream-unzip`` to decompress
           each entry as chunks arrive — no temporary file required and 
           memory usage stays constant.
```

**How it works:**
1. Download chunk of bytes → 2. Decompress chunk → 3. Write file chunk → 4. Repeat
5. **No intermediate ZIP buffer** stored on disk

**Memory Profile:**
```
[Download]  [Decompress]  [Write]
    ↓            ↓           ↓
  1MB chunk  → decompress → write
  1MB chunk  → decompress → write
  1MB chunk  → decompress → write
  ...
  
Total memory: ~1MB (constant, independent of ZIP size)
```

### Strategy 2: Buffering to Temp File
```python
streaming: When ``False``, streams into a temporary file on disk first, 
           then extracts with the stdlib ``zipfile`` module.
```

**How it works:**
1. Download entire ZIP → buffer to temp file
2. Once complete, extract with standard library
3. Delete temp file

**Disk/Memory Profile:**
```
[Download to Temp]     [Extract from Temp]
      ↓                        ↓
  2GB ZIP temp file  → buffer entire ZIP in memory/OS cache
  (stored on disk)     → extract all files
                       → delete temp file

Total disk: ~2GB (for temp file)
Total peak memory: ZIP size (or significant portion)
```

---

## Why Streaming? (The `stream-unzip` Library)

### The `stream-unzip` Advantage

**Without it (standard approach):**
```python
# Python stdlib - requires reading entire ZIP
with zipfile.ZipFile(filepath) as zf:
    zf.extractall(output_dir)  # Must buffer entire ZIP in memory!
```

**With `stream-unzip`:**
```python
from stream_unzip import stream_unzip

# Decompress as chunks arrive over the network
for zipped_path, _file_size, unzipped_chunks in stream_unzip(byte_chunks()):
    for chunk in unzipped_chunks:
        f.write(chunk)  # Write directly; don't buffer
```

### Benefits on UNO Q (Resource-Constrained Device)

1. **Memory Efficient**
   - ✅ Constant memory usage regardless of ZIP size
   - ✅ Can download 2GB models on devices with <512MB free RAM
   - ❌ Buffering would fail: allocating 2GB buffer on 4GB total memory = OOM

2. **Disk Space Efficient**
   - ✅ No temporary ZIP file → no extra disk space needed
   - ✅ Download + extract happens in one pass
   - ❌ Buffering needs 2x the file size (ZIP + extracted files)

3. **Faster**
   - ✅ Extraction starts while download still in progress (pipelining)
   - ✅ No wait for full download to complete before extracting
   - ❌ Buffering: download → wait → extract (sequential)

4. **Better Progress Reporting**
   - ✅ Can report download + extraction progress in real-time
   - ✅ User sees continuous progress (not "waiting" phases)

### Trade-offs

**Streaming (Default)**
- ✅ Lower memory
- ✅ Lower disk  
- ✅ Faster
- ❌ Requires external library (`stream-unzip`)
- ❌ More complex error handling (can't seek backward)

**Buffering (Fallback)**
- ✅ Uses Python stdlib (no extra dependency)
- ✅ Simpler error recovery (can re-read ZIP)
- ❌ Higher memory usage
- ❌ Requires 2x disk space temporarily

---

## Why Go Uses Buffering

Our Go implementation (`internal/orchestrator/modelsindex/custommodel/zip.go`) uses the **buffering approach**:

```go
// Buffer ZIP to temp file first
tmp, err := os.CreateTemp(destDir.String(), "model-download-*.zip")
if _, err := io.Copy(tmp, rc); err != nil {
    return nil, fmt.Errorf("buffer ZIP to disk: %w", err)
}

// Then extract with standard library
zr, err := zip.OpenReader(tmpPath)
```

### Why?

1. **Simplicity**
   - Go's standard `archive/zip` requires `io.ReaderAt` (random access)
   - Streaming requires implementing custom ZIP decompression
   - Not worth adding complexity for a MVP

2. **No Third-Party Dependency**
   - Go stdlib is sufficient
   - No need for `stream-unzip` equivalent

3. **Sufficient for Most Cases**
   - UNO Q has:
     - **Storage:** 64GB eMMC (plenty of temp space)
     - **RAM:** 4GB (enough for buffering typical models)
   - Streaming optimization becomes important only at >2GB sizes

### When Go Should Switch to Streaming

If we need to support:
- **Devices with <1GB free disk space**
- **Models >4GB in size**
- **Extremely memory-constrained environments**

At that point, we could implement streaming extraction using Go's `github.com/klauspost/compress/zip` or custom ZIP parser.

---

## Comparison Table

| Factor | Python (Streaming) | Go (Buffering) | UNO Q Suitable? |
|--------|-------------------|----------------|-----------------|
| **Memory Usage** | ~1MB constant | ZIP size (peak) | Both OK |
| **Disk Space** | 1x (ZIP size) | 2x (ZIP + extracted) | Go slightly higher |
| **Speed** | Faster (pipelined) | Slower (sequential) | Both acceptable |
| **Complexity** | Higher | Lower | Go wins |
| **Dependencies** | `stream-unzip` | stdlib only | Go wins |
| **Error Recovery** | Limited | Full re-read possible | Go wins |

---

## Architecture Decision: Progressive Enhancement

This represents a **progressive enhancement pattern**:

1. **MVP (Current Go):** Buffering with stdlib
   - ✅ Simple, works on UNO Q
   - ✅ No external dependencies
   - ✅ Sufficient for 99% of models

2. **Future (Optional):** Streaming extraction
   - 🎯 Only if we encounter memory/disk bottlenecks
   - 🎯 Would use Go compress libraries, not third-party
   - 🎯 Backwards compatible

The Python implementation is **production-hardened** for cloud-scale deployment where models can be arbitrarily large. Our Go implementation is **optimized for embedded systems** where simplicity and reliability matter more than memory efficiency.

---

## Recommendation for arduino-app-cli

**Keep the current buffering approach** because:

1. ✅ UNO Q has sufficient disk/memory
2. ✅ Simpler to maintain and test
3. ✅ Good error handling with temp files
4. ✅ Follows Go stdlib patterns
5. ✅ Can always add streaming later if needed

If we ever need streaming:
```go
// Future enhancement (not needed now)
// Use github.com/klauspost/compress/zip or implement custom parser
```
