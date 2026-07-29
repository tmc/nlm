// Package notebooklm provides a high-level client for NotebookLM.
//
// Client wraps the low-level batchexecute and gRPC-Web transports to expose
// notebooks, sources, chat, notes, research, and artifacts as ordinary Go
// methods. Construct a client with New; the zero value is not usable.
//
// Credentials are NotebookLM web-session values. This package does not launch
// a browser or refresh a session; callers own the credential lifecycle.
//
// NotebookLM does not publish a supported service API. This package uses the
// same private RPCs as the web application, so callers should expect service
// changes even when the Go API remains source compatible.
//
// Methods return generated protobuf values where preserving the wire model is
// useful and package-defined values where the client provides a higher-level
// projection. Typed sentinel errors such as ErrAuthExpired,
// ErrSourceCapReached, and ErrArtifactGenerating support errors.Is.
package notebooklm
