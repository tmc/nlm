package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/rpcinfo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// betoolOptions holds the parsed betool flags.
type betoolOptions struct {
	proto      bool     // --proto: decode into the bound proto message type
	verify     bool     // --verify: re-encode the proto to wire and report lossiness
	allMissing bool     // --verify-all: report every unmodeled position, not just the first
	infer      bool     // --infer: show a source-style schema for unmodeled fields
	asJSON     bool     // global --json: emit full JSON instead of the human-readable summary
	rpcID      string   // --rpc-id=<id>: override/disambiguate the rpc_id
	file       string   // positional input file, "" for stdin
	files      []string // infer-proto input files
	samplesDir string   // infer-proto corpus directory
}

// betool is a hidden developer command that translates raw batchexecute
// network payloads into a readable summary (or JSON) and back. It performs no network I/O
// and needs no authentication: it is a pure codec over the wire protocol,
// useful for inspecting HARs, hand-crafting requests, and round-trip testing.
//
// Modes:
//
//	decode-request   raw "f.req=...&at=...&" body        -> JSON WireRequest
//	encode-request   JSON WireRequest                    -> raw form body
//	decode-response  raw ")]}'"-prefixed response body   -> JSON WireResponse
//	encode-response  JSON WireResponse                   -> raw response body
//	infer-proto      one or more raw response payloads   -> merged descriptor
//
// With --proto, the decode modes resolve each call's rpc_id to its bound
// proto message type and emit canonical proto JSON (named fields) instead of
// the raw positional arrays. Use --rpc-id=<id> to supply or override the
// rpc_id (required when the wire data does not carry one, e.g. a response body
// with no id, or to disambiguate an rpc_id bound to more than one method).
//
// Input is read from a file argument, or from stdin when the argument is "-"
// or absent. Output goes to stdout.
func runBetool(args []string, jsonOutput bool) error {
	if len(args) == 0 {
		printBetoolUsage()
		return fmt.Errorf("betool: missing mode")
	}
	mode := args[0]
	rest := args[1:]

	switch mode {
	case "decode-request", "encode-request", "decode-response", "encode-response", "infer-proto":
	case "help", "-h", "--help":
		printBetoolUsage()
		return nil
	default:
		printBetoolUsage()
		return fmt.Errorf("betool: unknown mode %q", mode)
	}

	opts, err := parseBetoolFlags(mode, rest)
	if err != nil {
		return err
	}
	opts.asJSON = jsonOutput
	if mode == "infer-proto" {
		return betoolInferProto(opts)
	}
	input, err := readBetoolInput(opts.file)
	if err != nil {
		return err
	}

	switch mode {
	case "decode-request":
		return betoolDecodeRequest(input, opts)
	case "encode-request":
		return betoolEncodeRequest(input)
	case "decode-response":
		return betoolDecodeResponse(input, opts)
	case "encode-response":
		return betoolEncodeResponse(input)
	}
	return nil // unreachable
}

// parseBetoolFlags parses the flags and single optional input file that follow
// a betool mode. --proto and --rpc-id apply only to the decode modes.
func parseBetoolFlags(mode string, args []string) (betoolOptions, error) {
	var opts betoolOptions
	haveFile := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--proto":
			opts.proto = true
		case a == "--verify":
			opts.verify = true
		case a == "--verify-all":
			opts.verify = true
			opts.allMissing = true
		case a == "--infer" || a == "--infer-missing":
			if mode != "decode-response" && mode != "decode-request" {
				return opts, fmt.Errorf("betool %s: --infer-missing applies only to decode-request and decode-response", mode)
			}
			opts.infer = true
		case strings.HasPrefix(a, "--rpc-id="):
			opts.rpcID = strings.TrimPrefix(a, "--rpc-id=")
		case strings.HasPrefix(a, "--samples="):
			if mode != "infer-proto" {
				return opts, fmt.Errorf("betool %s: --samples applies only to infer-proto", mode)
			}
			opts.samplesDir = strings.TrimPrefix(a, "--samples=")
		case a == "--samples":
			if mode != "infer-proto" {
				return opts, fmt.Errorf("betool %s: --samples applies only to infer-proto", mode)
			}
			if i+1 >= len(args) {
				return opts, fmt.Errorf("betool %s: --samples requires a directory", mode)
			}
			i++
			opts.samplesDir = args[i]
		case strings.HasPrefix(a, "-") && a != "-":
			return opts, fmt.Errorf("betool %s: unknown flag %q", mode, a)
		default:
			if mode == "infer-proto" {
				opts.files = append(opts.files, a)
				continue
			}
			if haveFile {
				return opts, fmt.Errorf("betool %s: expected at most one input file", mode)
			}
			opts.file = a
			haveFile = true
		}
	}
	if opts.verify {
		opts.proto = true // --verify implies --proto
	}
	if opts.infer {
		opts.proto = true
		opts.verify = true
	}
	if mode == "infer-proto" && opts.rpcID == "" {
		return opts, fmt.Errorf("betool infer-proto: --rpc-id is required")
	}
	if (opts.proto || opts.rpcID != "") && mode != "decode-request" && mode != "decode-response" && mode != "infer-proto" {
		return opts, fmt.Errorf("betool %s: --proto/--rpc-id/--verify apply only to decode-request and decode-response", mode)
	}
	if mode == "infer-proto" && (opts.proto || opts.verify || opts.allMissing) {
		return opts, fmt.Errorf("betool infer-proto: --proto/--verify do not apply")
	}
	return opts, nil
}

// readBetoolInput reads the payload from file, or from stdin when file is ""
// or "-".
func readBetoolInput(file string) ([]byte, error) {
	if file == "" || file == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	return data, nil
}

func betoolDecodeRequest(input []byte, opts betoolOptions) error {
	req, err := batchexecute.DecodeRequest(string(input))
	if err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	if !opts.proto {
		if opts.asJSON {
			return writeJSON(req)
		}
		return writeRequestText(req)
	}
	// Emit one proto message per RPC, decoding its args into the bound request
	// type. Wrap each in an envelope so multi-RPC batches stay distinguishable.
	out := make([]protoEnvelope, 0, len(req.RPCs))
	for _, r := range req.RPCs {
		rpcID := r.ID
		if opts.rpcID != "" {
			rpcID = opts.rpcID
		}
		method, err := resolveMethod(rpcID)
		if err != nil {
			return err
		}
		msg := method.NewRequest()
		if err := beprotojson.Unmarshal(r.Args, msg); err != nil {
			if hint := nullInRepeatedHint(err, method.Request.Descriptor(), r.Args); hint != "" {
				return fmt.Errorf("rpc %s (%s): %s: %w", method.RPCID, method.FullName(), hint, err)
			}
			return fmt.Errorf("rpc %s (%s): unmarshal args into %s: %w",
				method.RPCID, method.FullName(), method.Request.Descriptor().FullName(), err)
		}
		env, err := newProtoEnvelope(method, string(method.Request.Descriptor().FullName()), msg)
		if err != nil {
			return err
		}
		if opts.verify {
			if err := verifyRoundTrip(&env, msg, r.Args, opts.allMissing); err != nil {
				return err
			}
		}
		if opts.infer {
			env.Inferred = inferMissingGroups(env.MissingGroups)
		}
		out = append(out, env)
	}
	if opts.asJSON {
		return writeJSON(out)
	}
	return writeProtoText(out)
}

func betoolEncodeRequest(input []byte) error {
	var req batchexecute.WireRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return fmt.Errorf("parse request JSON: %w", err)
	}
	body, err := batchexecute.EncodeRequest(&req)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	// Raw form bodies have no trailing newline on the wire; add one for
	// terminal readability without disturbing the payload.
	fmt.Fprintln(os.Stdout, body)
	return nil
}

func betoolDecodeResponse(input []byte, opts betoolOptions) error {
	resp, err := batchexecute.DecodeResponse(string(input))
	if err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !opts.proto {
		if opts.asJSON {
			return writeJSON(resp)
		}
		return writeResponseText(resp)
	}
	// Emit one proto message per response, decoding its data into the bound
	// response type.
	out := make([]protoEnvelope, 0, len(resp.Responses))
	for _, r := range resp.Responses {
		rpcID := r.ID
		if opts.rpcID != "" {
			rpcID = opts.rpcID
		}
		method, err := resolveMethod(rpcID)
		if err != nil {
			return err
		}
		msg := method.NewResponse()
		if len(r.Data) > 0 {
			if err := beprotojson.Unmarshal(r.Data, msg); err != nil {
				if hint := nullInRepeatedHint(err, method.Response.Descriptor(), r.Data); hint != "" {
					return fmt.Errorf("rpc %s (%s): %s: %w", method.RPCID, method.FullName(), hint, err)
				}
				return fmt.Errorf("rpc %s (%s): unmarshal data into %s: %w",
					method.RPCID, method.FullName(), method.Response.Descriptor().FullName(), err)
			}
		}
		env, err := newProtoEnvelope(method, string(method.Response.Descriptor().FullName()), msg)
		if err != nil {
			return err
		}
		if opts.verify {
			if err := verifyRoundTrip(&env, msg, r.Data, opts.allMissing); err != nil {
				return err
			}
		}
		if opts.infer {
			env.Inferred = inferMissingGroups(env.MissingGroups)
		}
		out = append(out, env)
	}
	if opts.asJSON {
		return writeJSON(out)
	}
	return writeProtoText(out)
}

// protoEnvelope wraps a decoded proto message with the metadata needed to
// interpret it. Message is raw proto JSON so writeJSON emits it inline (not as
// an escaped string). The RoundTrip fields are populated only under --verify.
type protoEnvelope struct {
	RPCID    string          `json:"rpc_id"`
	Method   string          `json:"method"`
	Type     string          `json:"type"`
	Message  json.RawMessage `json:"message"`
	Lossless *bool           `json:"roundtrip_lossless,omitempty"`
	// MissingCount is the total number of unmodeled/reshaped wire positions.
	// Always populated under --verify (0 when lossless).
	MissingCount int `json:"missing_field_count"`
	// MissingGroups collapses the findings by normalized structural path, so a
	// single gap repeated once per element reads as one group with a count.
	// This is the default --verify view.
	MissingGroups []deltaGroup `json:"missing_field_groups,omitempty"`
	// Missing lists every concrete finding, unabridged. Populated only under
	// --verify-all, since it can be large.
	Missing  []fieldDelta `json:"missing_fields,omitempty"`
	Inferred string       `json:"inferred,omitempty"`
}

// verifyRoundTrip re-encodes msg back to the batchexecute wire form and diffs
// it against the original wire payload, guided by the proto descriptor. It
// records whether the proto view is lossless and, when it is not, the wire
// positions the descriptor fails to model (missing_fields) — turning a proto
// schema gap into an actionable finding rather than a silent data loss.
//
// By default findings are grouped by normalized structural path (array indices
// collapsed to "*"), so a single gap repeated once per element reads as one
// group with a count rather than N near-identical entries; allMissing also
// attaches the full unabridged list.
func verifyRoundTrip(env *protoEnvelope, msg proto.Message, originalWire json.RawMessage, allMissing bool) error {
	deltas, err := diffWireAgainstProto(originalWire, msg)
	if err != nil {
		return fmt.Errorf("rpc %s (%s): diff wire: %w", env.RPCID, env.Method, err)
	}
	lossless := len(deltas) == 0
	env.Lossless = &lossless
	env.MissingCount = len(deltas)
	if len(deltas) > 0 {
		env.MissingGroups = groupDeltas(deltas)
	}
	if allMissing {
		env.Missing = deltas
	}
	return nil
}

// newProtoEnvelope renders msg to canonical proto JSON and wraps it with the
// method's true rpc_id, full name, and message type.
func newProtoEnvelope(method rpcinfo.Method, typeName string, msg proto.Message) (protoEnvelope, error) {
	// UseProtoNames keeps the wire-style snake_case field names; EmitUnpopulated
	// is off so the output stays compact and readable.
	b, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(msg)
	if err != nil {
		return protoEnvelope{}, fmt.Errorf("rpc %s (%s): marshal proto JSON: %w", method.RPCID, method.FullName(), err)
	}
	return protoEnvelope{
		RPCID:   method.RPCID,
		Method:  method.FullName(),
		Type:    typeName,
		Message: json.RawMessage(b),
	}, nil
}

// resolveMethod looks up the proto method bound to sel, which is either an
// rpc_id or, to disambiguate an rpc_id shared by several methods, a method
// name ("CreateAudioOverview" or "Service.CreateAudioOverview"). It surfaces
// actionable errors for the unknown and ambiguous cases.
func resolveMethod(sel string) (rpcinfo.Method, error) {
	if sel == "" {
		return rpcinfo.Method{}, fmt.Errorf("no rpc_id available; pass --rpc-id=<id> (the payload carries no id)")
	}
	m, err := rpcinfo.Lookup(sel)
	if err == nil {
		return m, nil
	}
	// The selector may instead be a method name used to disambiguate a shared
	// rpc_id (e.g. --rpc-id=CreateAudioOverview for R7cb6c). Try that before
	// giving up, but preserve the original ambiguity error otherwise.
	var unknown rpcinfo.ErrUnknownRPCID
	if errors.As(err, &unknown) {
		if byName, nameErr := rpcinfo.LookupByName(sel); nameErr == nil {
			return byName, nil
		}
	}
	return rpcinfo.Method{}, err
}

func betoolEncodeResponse(input []byte) error {
	var resp batchexecute.WireResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return fmt.Errorf("parse response JSON: %w", err)
	}
	raw, err := batchexecute.EncodeResponse(&resp)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	fmt.Fprintln(os.Stdout, raw)
	return nil
}

// writeJSON writes v as indented JSON to stdout without HTML-escaping, so that
// wire payloads containing <, >, and & are reproduced faithfully.
func writeJSON(v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	_, err := os.Stdout.Write(buf.Bytes())
	return err
}

func printBetoolUsage() {
	fmt.Fprint(os.Stderr, strings.TrimLeft(`
usage: nlm betool <mode> [flags] [file]

Translate raw batchexecute network payloads to a readable summary or JSON, and
back. Reads from [file], or from stdin when [file] is "-" or omitted. Performs
no network I/O.

Modes:
  decode-request    raw "f.req=...&at=...&" body      -> text (--json for JSON)
  encode-request    JSON request spec                 -> raw form body
  decode-response   raw ")]}'"-prefixed response body -> text (--json for JSON)
  encode-response   JSON response spec                -> raw response body
  infer-proto       raw response payloads             -> descriptor textproto

infer-proto flags:
  --rpc-id=<id>     select the response descriptor; required for inference
  --samples=<dir>   infer from every regular file in a directory
                    (multiple input files may also be listed; raw responses,
                    HAR, JSONL traffic, and httprr recordings are accepted)
  --json            emit FileDescriptorProto as protojson instead of textproto

Decode modes print a human-readable summary by default; pass the global --json
flag (before the mode: "nlm --json betool decode-response …") for the full
structured output. The encode modes consume that JSON, so round-tripping a
payload needs --json on the decode side.

Flags (decode modes only):
  --proto           decode into the proto message type bound to the rpc_id,
                    showing proto JSON with named fields
  --rpc-id=<id>     supply or override the rpc_id, or a method name to
                    disambiguate a shared rpc_id (e.g. CreateVideoOverview)
  --verify          (implies --proto) re-encode the proto back to wire and
                    report whether the round-trip is lossless, plus the wire
                    positions the proto type does not model, grouped by
                    normalized path (with --json: "roundtrip_lossless",
                    "missing_field_count", "missing_field_groups")
  --verify-all      (implies --verify) also attach the full unabridged list of
                    findings ("missing_fields")
	  --infer-missing   (alias: --infer; implies --verify) show inferred missing fields as a
                    compact source-style proto fragment

Examples:
  # Inspect a request captured from a HAR:
  pbpaste | nlm betool decode-request

  # Decode a response into its typed proto message:
  nlm betool decode-response --proto resp.txt

  # A response body has no rpc_id, so supply it:
  nlm betool decode-response --proto --rpc-id=CCqFvf resp.txt

  # Round-trip a response body (encode consumes JSON, so decode with --json):
  nlm --json betool decode-response resp.txt | nlm betool encode-response

  # Hand-craft a request body from JSON:
  echo '{"rpcs":[{"id":"wXbhsf","args":[]}],"at":"TOKEN"}' \
    | nlm betool encode-request
`, "\n"))
}
