// Package beprotojson provides functionality to marshal and unmarshal protocol buffer
// messages in Google's batchexecute-style JSON array format.
//
// The batchexecute format uses positional arrays instead of named fields, where
// the position in the array corresponds to the protocol buffer field number.
//
// This package aims to be API-compatible with google.golang.org/protobuf/encoding/protojson
// while handling the specialized batchexecute format.
package beprotojson
