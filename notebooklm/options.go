package notebooklm

import (
	"net/http"

	"github.com/tmc/nlm/internal/authuser"
	"github.com/tmc/nlm/internal/batchexecute"
)

// Credentials contains the credentials used to authenticate NotebookLM
// requests.
type Credentials struct {
	AuthToken string
	Cookies   string
}

type clientConfig struct {
	Debug             bool
	DebugParsing      bool
	DebugFieldMapping bool
	UseDirectRPC      bool
	SkipSources       bool
	AuthUser          string
	batchOptions      []batchexecute.Option
}

// Option configures a Client.
type Option func(*clientConfig)

// WithDebug enables client and RPC debug output.
func WithDebug(debug bool) Option {
	return func(config *clientConfig) {
		config.Debug = debug
	}
}

// WithProtoDebug enables protobuf parsing diagnostics.
func WithProtoDebug(debugParsing, debugFieldMapping bool) Option {
	return func(config *clientConfig) {
		config.DebugParsing = debugParsing
		config.DebugFieldMapping = debugFieldMapping
	}
}

// WithUseDirectRPC selects direct RPC implementations where available.
func WithUseDirectRPC(use bool) Option {
	return func(config *clientConfig) {
		config.UseDirectRPC = use
	}
}

// WithSkipSources disables automatic project-source lookup for chat requests.
func WithSkipSources(skip bool) Option {
	return func(config *clientConfig) {
		config.SkipSources = skip
	}
}

// WithAuthUser sets the Google account index for multi-account profiles.
func WithAuthUser(authUser string) Option {
	return func(config *clientConfig) {
		config.AuthUser = authuser.Normalize(authUser)
	}
}

// WithHTTPClient sets the HTTP client used for batchexecute requests.
func WithHTTPClient(client *http.Client) Option {
	return func(config *clientConfig) {
		config.batchOptions = append(config.batchOptions, batchexecute.WithHTTPClient(client))
	}
}

// WithURLParams adds URL parameters to batchexecute requests.
func WithURLParams(params map[string]string) Option {
	return func(config *clientConfig) {
		config.batchOptions = append(config.batchOptions, batchexecute.WithURLParams(params))
	}
}
