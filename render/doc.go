// Package render turns NotebookLM notes, sources, and citations into
// terminal text, Markdown, or self-contained HTML.
//
// It is the public face of the rendering used by the nlm command line, so a
// program built on the notebooklm client can present the same output without
// reimplementing citation resolution, rich-document flattening, or the
// offset-gap handling that separates a readable view from the byte-faithful
// [notebooklm.LoadSourceText.Full].
//
// The functions take a notebooklm value and an [io.Writer]; the intermediate
// document models stay internal.
package render
