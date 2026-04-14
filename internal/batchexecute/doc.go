// Package batchexecute implements the Google batchexecute RPC protocol.
//
// Batchexecute is Google's internal protocol for batching multiple RPC calls
// into a single HTTP request. It encodes arguments as positional JSON arrays
// and wraps responses in a chunked envelope format.
package batchexecute
