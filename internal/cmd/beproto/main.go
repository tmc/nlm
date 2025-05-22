// Package main provides a utility for marshaling/unmarshaling
// between Protocol Buffers and NDJSON formats.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func main() {
	// Define command line flags
	mode := flag.String("mode", "unmarshal", "Mode: 'marshal', 'unmarshal', 'record'")
	messageType := flag.String("type", "project", "Proto message type: 'project', 'source', 'note'")
	raw := flag.Bool("raw", false, "Raw mode (don't try to parse as specific message type)")
	help := flag.Bool("help", false, "Show help")
	debug := flag.Bool("debug", false, "Enable debug output")
	recordMode := flag.Bool("record", false, "Record API requests and responses")

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "Running in %s mode with message type %s\n", *mode, *messageType)
	}

	// Special handling for record/replay command
	if *recordMode || strings.ToLower(*mode) == "record" {
		fmt.Println("Running in record/replay mode...")
		if err := recordAndReplayListProjects(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch strings.ToLower(*mode) {
	case "marshal":
		if err := marshal(os.Stdin, os.Stdout, *messageType, *raw, *debug); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "unmarshal":
		if err := unmarshal(os.Stdin, os.Stdout, *messageType, *raw, *debug); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown mode: %s\n", *mode)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("BeProto - Utility for marshaling/unmarshaling between Protocol Buffers and NDJSON")
	fmt.Println("\nUsage:")
	fmt.Println("  beproto [flags]")
	fmt.Println("\nFlags:")
	fmt.Println("  -mode string    Mode: 'marshal', 'unmarshal', or 'record' (default \"unmarshal\")")
	fmt.Println("  -type string    Proto message type: 'project', 'source', 'note' (default \"project\")")
	fmt.Println("  -raw            Raw mode (don't try to parse as specific message type)")
	fmt.Println("  -record         Record API requests and responses")
	fmt.Println("  -help           Show this help message")
	fmt.Println("  -debug          Enable debug output")
	fmt.Println("\nExamples:")
	fmt.Println("  # Unmarshal Project Protocol Buffer data to JSON")
	fmt.Println("  cat data.proto | beproto -mode unmarshal -type project > data.json")
	fmt.Println("\n  # Marshal JSON data to Project Protocol Buffer")
	fmt.Println("  cat data.json | beproto -mode marshal -type project > data.proto")
	fmt.Println("\n  # Handle raw binary data without specific message type")
	fmt.Println("  cat data.proto | beproto -mode unmarshal -raw > data.json")
	fmt.Println("\n  # Record and replay API calls (requires NLM_AUTH_TOKEN and NLM_COOKIES env vars)")
	fmt.Println("  beproto -record")
	fmt.Println("  beproto -mode record")
}

// createProtoMessage creates a new proto message based on type
func createProtoMessage(messageType string) (proto.Message, error) {
	switch strings.ToLower(messageType) {
	case "project", "notebook":
		return &pb.Project{}, nil
	case "source", "note":
		return &pb.Source{}, nil
	case "projectlist", "notebooklist":
		return &pb.ListRecentlyViewedProjectsResponse{}, nil
	default:
		return nil, fmt.Errorf("unknown message type: %s", messageType)
	}
}

// unmarshal converts Protocol Buffer data from reader to JSON and writes to writer
func unmarshal(r io.Reader, w io.Writer, messageType string, raw bool, debug bool) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	if debug {
		fmt.Fprintf(os.Stderr, "Input data (%d bytes)\n", len(data))
		if len(data) < 200 {
			fmt.Fprintf(os.Stderr, "Raw data: %q\n", string(data))
		}
	}

	// Handle empty input
	if len(data) == 0 {
		fmt.Fprintln(os.Stderr, "Warning: Empty input received")
		return nil
	}

	// Early preprocessing - detect and strip )]}'
	strData := string(data)
	if strings.HasPrefix(strData, ")]}'") {
		if debug {
			fmt.Fprintf(os.Stderr, "Detected and removing )]}' prefix\n")
		}
		strData = strings.TrimPrefix(strData, ")]}'")
		data = []byte(strData)
	}

	// Handle raw mode
	if raw {
		// Try to unmarshal as generic JSON
		var jsonData interface{}
		if err := json.Unmarshal(data, &jsonData); err != nil {
			return fmt.Errorf("unmarshal JSON: %w", err)
		}

		// Marshal to pretty JSON
		jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal JSON: %w", err)
		}

		if debug {
			fmt.Fprintf(os.Stderr, "Unmarshaled to JSON (%d bytes)\n", len(jsonBytes))
		}

		_, err = w.Write(jsonBytes)
		if err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		
		// Add final newline
		fmt.Fprintln(w)
		return nil
	}

	// Create appropriate proto message
	msg, err := createProtoMessage(messageType)
	if err != nil {
		return err
	}

	// Try to unmarshal the protocol buffer
	if err := proto.Unmarshal(data, msg); err != nil {
		// If binary parsing failed, try to parse as JSON
		if err := protojson.Unmarshal(data, msg); err != nil {
			return fmt.Errorf("unmarshal proto: %w", err)
		}
	}

	// Marshal to JSON
	marshaler := protojson.MarshalOptions{
		Indent:          "  ",
		EmitUnpopulated: true,
		UseProtoNames:   true,
	}
	jsonData, err := marshaler.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal to JSON: %w", err)
	}
	if _, err := w.Write(jsonData); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	// Add final newline
	fmt.Fprintln(w)
	return nil
}

// marshal converts JSON data from reader to Protocol Buffer and writes to writer
func marshal(r io.Reader, w io.Writer, messageType string, raw bool, debug bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // Set max buffer size to 10MB

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		
		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		if debug {
			fmt.Fprintf(os.Stderr, "Processing line %d (%d bytes)\n", lineNum, len(line))
		}

		// Create appropriate proto message
		msg, err := createProtoMessage(messageType)
		if err != nil {
			return err
		}

		// Parse JSON to proto message
		unmarshaler := protojson.UnmarshalOptions{
			AllowPartial:   true,
			DiscardUnknown: true,
		}
		if err := unmarshaler.Unmarshal([]byte(line), msg); err != nil {
			return fmt.Errorf("line %d: parse JSON: %w", lineNum, err)
		}

		// Marshal to protocol buffer
		protoBytes, err := proto.Marshal(msg)
		if err != nil {
			return fmt.Errorf("line %d: marshal to proto: %w", lineNum, err)
		}

		if debug {
			fmt.Fprintf(os.Stderr, "Marshaled to proto (%d bytes)\n", len(protoBytes))
		}

		// Write the protocol buffer
		_, err = w.Write(protoBytes)
		if err != nil {
			return fmt.Errorf("line %d: write output: %w", lineNum, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	return nil
}