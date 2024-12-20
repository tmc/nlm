package api

// Option configures a Client
type Option func(*Client)

// WithHost sets the API host
func WithHost(host string) Option {
    return func(c *Client) {
        c.host = host
    }
}

// WithApp sets the API app name
func WithApp(app string) Option {
    return func(c *Client) {
        c.app = app
    }
}

// WithBuildID sets the API build ID
func WithBuildID(buildID string) Option {
    return func(c *Client) {
        c.buildID = buildID
    }
}

// WithLanguage sets the API language
func WithLanguage(lang string) Option {
    return func(c *Client) {
        c.language = lang
    }
}

// WithHeader adds a custom header
func WithHeader(key, value string) Option {
    return func(c *Client) {
        c.headers[key] = value
    }
}

// WithURLParam adds a custom URL parameter
func WithURLParam(key, value string) Option {
    return func(c *Client) {
        c.urlParams[key] = value
    }
}

// SourceOption configures source operations
type SourceOption func(*sourceOptions)

type sourceOptions struct {
    name        string
    base64      bool
    contentType string
    noType      bool
    autoType    bool
}

// WithSourceName sets the source name
func WithSourceName(name string) SourceOption {
    return func(o *sourceOptions) {
        o.name = name
    }
}

// WithBase64Encoding forces base64 encoding
func WithBase64Encoding() SourceOption {
    return func(o *sourceOptions) {
        o.base64 = true
    }
}

// WithContentType sets explicit content type
func WithContentType(ct string) SourceOption {
    return func(o *sourceOptions) {
        o.contentType = ct
        o.noType = false
        o.autoType = false
    }
}

// WithContentTypeNone disables content type
func WithContentTypeNone() SourceOption {
    return func(o *sourceOptions) {
        o.noType = true
        o.autoType = false
        o.contentType = ""
    }
}

// WithContentTypeAuto enables automatic content type detection
func WithContentTypeAuto() SourceOption {
    return func(o *sourceOptions) {
        o.autoType = true
        o.noType = false
        o.contentType = ""
    }
}

