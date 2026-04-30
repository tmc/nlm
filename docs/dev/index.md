---
title: Developer Guide
---
# Developer Guide

Internal documentation for contributors to the nlm project.

## Architecture

nlm is structured in layers:

- **cmd/nlm** — CLI entry point, flag parsing, command dispatch
- **internal/notebooklm/api** — High-level API client (notebooks, sources, chat, audio, etc.)
- **internal/notebooklm/rpc** — Low-level RPC client wrapping batchexecute
- **internal/batchexecute** — Google batchexecute protocol implementation
- **internal/beprotojson** — Custom JSON-to-protobuf unmarshaling for Google's wire format
- **internal/nlmmcp** — MCP server exposing API operations
- **internal/auth** — Browser automation for cookie extraction
- **gen/method** — Generated RPC request encoders; safe to regenerate
- **internal/method** — HAR-verified RPC request encoders that must not be regenerated
- **gen/service** — Generated orchestration service clients
- **gen/notebooklm/v1alpha1** — Protobuf type definitions

## Key concepts

### batchexecute protocol

NotebookLM uses Google's internal `batchexecute` RPC protocol. Requests are URL-encoded arrays of nested JSON; responses are either chunked streams or JSON arrays. Each RPC is identified by a short string ID (e.g., `o4cbdc` for source upload, `tr032e` for source processing).

### Wire format encoders

The `gen/method/` directory contains generated encoders. HAR-verified,
hand-written encoders live in `internal/method/` so `buf generate` cannot
overwrite them. Each hand-written encoder has a guard comment:

```
// Wire format verified against HAR capture — do not regenerate.
```

### Authentication

nlm uses session cookies extracted from the browser via Chrome DevTools Protocol. The key cookies are SID, HSID, SSID, APISID, and SAPISID. SAPISIDHASH is computed from SAPISID for API authentication.

## Internal docs

- [Remaining Gaps](remaining-gaps.md) — Live status of every command surface; the source of truth for what's open
- [Codegen Split](codegen.md) — Why HAR-verified encoders live under `internal/method/`, not `gen/method/`
- [Test Conventions](test-conventions.md) — `testdata/` vs `docs/captures/`, fixture-skip pattern, encoder guard comments
- [HTTP Capture](http-capture.md) — Testing framework for recording and replaying HTTP
- [Interactive Audio Spec](spec-interactive-audio.md) — WebRTC bidirectional audio wire-format reference
