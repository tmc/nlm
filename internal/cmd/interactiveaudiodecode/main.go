package main

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	ia "github.com/tmc/nlm/internal/interactiveaudio"
)

type options struct {
	capture string
	base64  string
	hex     string
	from    int
	to      int
	verbose bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "interactiveaudiodecode: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}

	switch {
	case opts.capture != "":
		return decodeCapture(opts, stdout)
	case opts.base64 != "":
		frame, raw, err := decodeBase64Input(opts.base64)
		if err != nil {
			return err
		}
		return writeFrame(stdout, 0, raw, frame, opts.verbose)
	case opts.hex != "":
		frame, raw, err := decodeHexInput(opts.hex)
		if err != nil {
			return err
		}
		return writeFrame(stdout, 0, raw, frame, opts.verbose)
	default:
		return errors.New("missing payload source")
	}
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var opts options

	flags := flag.NewFlagSet("interactiveaudiodecode", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.capture, "capture", "", "decode a jsonl capture file")
	flags.StringVar(&opts.base64, "base64", "", "decode one base64-encoded frame")
	flags.StringVar(&opts.hex, "hex", "", "decode one hex-encoded frame")
	flags.IntVar(&opts.from, "from", -1, "minimum sequence number to print")
	flags.IntVar(&opts.to, "to", -1, "maximum sequence number to print")
	flags.BoolVar(&opts.verbose, "verbose", false, "print raw payload bytes and parsed fields")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: interactiveaudiodecode [flags] [payload]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Decode interactive-audio DataChannel frames from a capture or a raw payload.")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Flags:")
		flags.PrintDefaults()
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "If no source flag is provided, one positional payload is accepted and auto-detected as base64 or hex.")
	}

	if err := flags.Parse(args); err != nil {
		return options{}, err
	}

	positional := flags.Args()
	if len(positional) > 1 {
		return options{}, errors.New("accepts at most one positional payload")
	}
	if len(positional) == 1 {
		if opts.capture != "" || opts.base64 != "" || opts.hex != "" {
			return options{}, errors.New("positional payload cannot be combined with source flags")
		}
		payload := strings.TrimSpace(positional[0])
		if looksLikeHex(payload) {
			opts.hex = payload
		} else {
			opts.base64 = payload
		}
	}

	sources := 0
	for _, src := range []string{opts.capture, opts.base64, opts.hex} {
		if strings.TrimSpace(src) != "" {
			sources++
		}
	}
	if sources != 1 {
		return options{}, errors.New("provide exactly one of -capture, -base64, -hex, or a positional payload")
	}

	if opts.from >= 0 && opts.to >= 0 && opts.from > opts.to {
		return options{}, errors.New("-from must be less than or equal to -to")
	}

	return opts, nil
}

func decodeCapture(opts options, stdout io.Writer) error {
	f, err := os.Open(opts.capture)
	if err != nil {
		return fmt.Errorf("open capture: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)
	for idx := 0; scanner.Scan(); idx++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		text, err := captureText([]byte(raw))
		if err != nil {
			return fmt.Errorf("decode capture line %d: %w", idx+1, err)
		}
		if text == "" {
			continue
		}
		frame, outer, err := decodeBase64Input(text)
		if err != nil {
			return fmt.Errorf("decode capture line %d: %w", idx+1, err)
		}
		if opts.from >= 0 && frame.SequenceNumber < opts.from {
			continue
		}
		if opts.to >= 0 && frame.SequenceNumber > opts.to {
			continue
		}
		if err := writeFrame(stdout, idx, outer, frame, opts.verbose); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan capture: %w", err)
	}
	return nil
}

func decodeBase64Input(text string) (ia.Frame, []byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil {
		return ia.Frame{}, nil, fmt.Errorf("decode base64 payload: %w", err)
	}
	frame, err := ia.DecodeFrame(raw)
	if err != nil {
		return ia.Frame{}, nil, fmt.Errorf("decode frame: %w", err)
	}
	return frame, raw, nil
}

func decodeHexInput(text string) (ia.Frame, []byte, error) {
	clean := strings.TrimSpace(text)
	clean = strings.TrimPrefix(clean, "0x")
	clean = strings.ReplaceAll(clean, " ", "")
	raw, err := hex.DecodeString(clean)
	if err != nil {
		return ia.Frame{}, nil, fmt.Errorf("decode hex payload: %w", err)
	}
	frame, err := ia.DecodeFrame(raw)
	if err != nil {
		return ia.Frame{}, nil, fmt.Errorf("decode frame: %w", err)
	}
	return frame, raw, nil
}

func writeFrame(w io.Writer, index int, outer []byte, frame ia.Frame, verbose bool) error {
	if _, err := fmt.Fprintf(w, "idx=%d seq=%d size=%d type=%s %s\n",
		index,
		frame.SequenceNumber,
		frame.PayloadSize,
		frameType(frame),
		frameSummary(frame.Event),
	); err != nil {
		return err
	}
	if !verbose {
		return nil
	}
	if len(outer) > 0 {
		if _, err := fmt.Fprintf(w, "  outer=%x\n", outer); err != nil {
			return err
		}
	}
	if len(frame.RawPayload) > 0 {
		if _, err := fmt.Fprintf(w, "  payload=%x\n", frame.RawPayload); err != nil {
			return err
		}
	}
	for _, field := range verboseFields(frame.Event) {
		if _, err := fmt.Fprintf(w, "  field=%s\n", field); err != nil {
			return err
		}
	}
	return nil
}

func frameType(frame ia.Frame) string {
	if frame.Event == nil {
		return "UNKNOWN"
	}
	return frame.Event.MessageType().String()
}

func frameSummary(event ia.Event) string {
	switch e := event.(type) {
	case ia.UserUtterance:
		return fmt.Sprintf("final=%t text=%s utterance=%q", e.IsFinal, quoteText(e.Transcript), e.UtteranceID)
	case ia.AgentUtterance:
		return fmt.Sprintf("final=%t speakers=%s text=%s utterance=%q", e.IsFinal, joinQuoted(e.Speakers), quoteText(e.Transcript), e.UtteranceID)
	case ia.TTSEvent:
		return fmt.Sprintf("event=%d utterance=%q segment=%d timestamp=%d.%09d", e.EventType, e.UtteranceID, e.SegmentIdx, e.Timestamp.EpochSeconds, e.Timestamp.Nanos)
	case ia.SendAudioEvent:
		return fmt.Sprintf("trigger=%d utterance=%q", e.TriggerType, e.UtteranceID)
	case ia.PlaybackEvent:
		return fmt.Sprintf("states=%s", joinQuoted(e.States))
	case ia.MicrophoneEvent:
		return fmt.Sprintf("status=%d", e.StatusCode)
	case ia.StatusMessage:
		return fmt.Sprintf("text=%s", quoteText(e.Text))
	case ia.UnknownEvent:
		return fmt.Sprintf("raw=%x", e.Raw)
	case nil:
		return "empty"
	default:
		return fmt.Sprintf("%T", event)
	}
}

func verboseFields(event ia.Event) []string {
	switch e := event.(type) {
	case ia.StatusMessage:
		return stringifyFields(e.Fields)
	case ia.UnknownEvent:
		return stringifyFields(e.Fields)
	default:
		return nil
	}
}

func stringifyFields(fields []ia.WireField) []string {
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, field.String())
	}
	return out
}

func quoteText(text string) string {
	return strconv.Quote(shorten(strings.TrimSpace(text), 120))
}

func joinQuoted(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, strconv.Quote(item))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func shorten(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}

func looksLikeHex(text string) bool {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "0x")
	text = strings.ReplaceAll(text, " ", "")
	if text == "" || len(text)%2 != 0 {
		return false
	}
	for _, r := range text {
		switch {
		case '0' <= r && r <= '9':
		case 'a' <= r && r <= 'f':
		case 'A' <= r && r <= 'F':
		default:
			return false
		}
	}
	return true
}

type captureLine struct {
	Data     string `json:"data"`
	Response struct {
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"response"`
}

func captureText(line []byte) (string, error) {
	var entry captureLine
	if err := json.Unmarshal(line, &entry); err != nil {
		return "", err
	}
	if entry.Data != "" {
		return entry.Data, nil
	}
	return entry.Response.Content.Text, nil
}
