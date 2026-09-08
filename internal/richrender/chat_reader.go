package richrender

import _ "embed"

// The reader assets are embedded into each rendered page so saved conversations
// keep their navigation and citation controls without a local asset server.

//go:embed chat_reader.css
var chatReaderCSS string

//go:embed chat_reader.js
var chatReaderScript string
