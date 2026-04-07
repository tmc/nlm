package interactiveaudio

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// DecodeFrame decodes one raw protobuf DataChannel frame.
func DecodeFrame(payload []byte) (Frame, error) {
	outer, err := parseFields(payload)
	if err != nil {
		return Frame{}, fmt.Errorf("decode outer envelope: %w", err)
	}

	innerBytes := fieldBytes(outer, 2)
	if len(innerBytes) == 0 {
		return Frame{}, fmt.Errorf("decode outer envelope: missing field 2")
	}

	inner, err := parseFields(innerBytes[0])
	if err != nil {
		return Frame{}, fmt.Errorf("decode inner envelope: %w", err)
	}

	frame := Frame{
		SequenceNumber: int(firstInt(inner, 1)),
		PayloadSize:    int(firstInt(inner, 2)),
	}

	messageBytes := fieldBytes(inner, 4)
	if len(messageBytes) == 0 {
		return frame, nil
	}
	frame.RawPayload = append([]byte(nil), messageBytes[0]...)

	event, err := decodeEvent(messageBytes[0])
	if err != nil {
		return Frame{}, err
	}
	frame.Event = event
	return frame, nil
}

// DecodeBase64Frame decodes a base64-encoded capture frame.
func DecodeBase64Frame(text string) (Frame, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil {
		return Frame{}, fmt.Errorf("decode base64 frame: %w", err)
	}
	return DecodeFrame(b)
}

// DecodeCaptureFile decodes a jsonl capture file into Frames.
func DecodeCaptureFile(path string) ([]Frame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open capture file: %w", err)
	}
	defer f.Close()

	var frames []Frame
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		text, err := captureFrameText([]byte(line))
		if err != nil {
			return nil, err
		}
		if text == "" {
			continue
		}

		frame, err := DecodeBase64Frame(text)
		if err != nil {
			return nil, fmt.Errorf("decode capture frame: %w", err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan capture file: %w", err)
	}
	return frames, nil
}

type captureLine struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	Response struct {
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"response"`
}

func captureFrameText(line []byte) (string, error) {
	var entry captureLine
	if err := json.Unmarshal(line, &entry); err != nil {
		return "", fmt.Errorf("decode capture line: %w", err)
	}
	if entry.Data != "" {
		return entry.Data, nil
	}
	return entry.Response.Content.Text, nil
}

func decodeEvent(payload []byte) (Event, error) {
	fields, err := parseFields(payload)
	if err != nil {
		return nil, fmt.Errorf("decode event payload: %w", err)
	}
	if len(fields) == 0 {
		return UnknownEvent{Raw: append([]byte(nil), payload...)}, nil
	}

	switch fields[0].number {
	case 1:
		return decodeUserUtterance(fields[0].value)
	case 2:
		return decodeAgentUtterance(fields[0].value)
	case 3:
		return decodeTTSEvent(fields[0].value)
	case 4:
		return decodeSendAudioEvent(fields[0].value)
	case 6:
		return decodePlaybackEvent(fields[0].value)
	case 7:
		return decodeMicrophoneEvent(fields[0].value)
	case 8:
		return decodeStatusMessage(fields[0].value)
	default:
		return UnknownEvent{
			Type:   pb.AgentCommsMessageType(fields[0].number),
			Raw:    append([]byte(nil), payload...),
			Fields: toWireFields(fields),
		}, nil
	}
}

func decodeUserUtterance(b []byte) (Event, error) {
	fields, err := parseFields(b)
	if err != nil {
		return nil, fmt.Errorf("decode user utterance: %w", err)
	}
	return UserUtterance{
		Transcript:  firstString(fields, 3),
		IsFinal:     firstBool(fields, 4),
		UtteranceID: firstString(fields, 5),
	}, nil
}

func decodeAgentUtterance(b []byte) (Event, error) {
	fields, err := parseFields(b)
	if err != nil {
		return nil, fmt.Errorf("decode agent utterance: %w", err)
	}
	return AgentUtterance{
		SpeakerID:   int32(firstInt(fields, 1)),
		Speakers:    fieldStringList(fields, 2),
		Transcript:  firstString(fields, 3),
		IsFinal:     firstBool(fields, 4),
		UtteranceID: firstString(fields, 5),
	}, nil
}

func decodeTTSEvent(b []byte) (Event, error) {
	fields, err := parseFields(b)
	if err != nil {
		return nil, fmt.Errorf("decode tts event: %w", err)
	}

	tsFields, ok, err := fieldMessage(fields, 4)
	if err != nil {
		return nil, fmt.Errorf("decode tts timestamp: %w", err)
	}
	ts := TTSTimestamp{}
	if ok {
		ts.EpochSeconds = firstInt(tsFields, 1)
		ts.Nanos = int32(firstInt(tsFields, 2))
	}

	return TTSEvent{
		EventType:   int32(firstInt(fields, 1)),
		UtteranceID: firstString(fields, 2),
		SegmentIdx:  int32(firstInt(fields, 3)),
		Timestamp:   ts,
	}, nil
}

func decodeSendAudioEvent(b []byte) (Event, error) {
	fields, err := parseFields(b)
	if err != nil {
		return nil, fmt.Errorf("decode send-audio event: %w", err)
	}
	return SendAudioEvent{
		TriggerType: int32(firstInt(fields, 1)),
		UtteranceID: firstString(fields, 2),
	}, nil
}

func decodePlaybackEvent(b []byte) (Event, error) {
	fields, err := parseFields(b)
	if err != nil {
		return nil, fmt.Errorf("decode playback event: %w", err)
	}
	states := fieldStringList(fields, 1)
	states = append(states, fieldStringList(fields, 2)...)
	states = append(states, fieldStringList(fields, 3)...)
	return PlaybackEvent{States: states}, nil
}

func decodeMicrophoneEvent(b []byte) (Event, error) {
	fields, err := parseFields(b)
	if err != nil {
		return nil, fmt.Errorf("decode microphone event: %w", err)
	}
	return MicrophoneEvent{StatusCode: int32(firstInt(fields, 1))}, nil
}

func decodeStatusMessage(b []byte) (Event, error) {
	fields, err := parseFields(b)
	if err == nil && len(fields) > 0 {
		if text := firstString(fields, 1); text != "" {
			return StatusMessage{
				Text:   text,
				Raw:    append([]byte(nil), b...),
				Fields: toWireFields(fields),
			}, nil
		}
	}

	text := ""
	if s, ok := wireString(b); ok {
		text = strings.TrimSpace(s)
	}
	if text == "" {
		text = hexString(b)
	}

	return StatusMessage{
		Text:   text,
		Raw:    append([]byte(nil), b...),
		Fields: toWireFields(fields),
	}, nil
}

// DecodeCapture reads frames from a capture stream.
func DecodeCapture(r io.Reader) ([]Frame, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)

	var frames []Frame
	for scanner.Scan() {
		text, err := captureFrameText(scanner.Bytes())
		if err != nil {
			return nil, err
		}
		if text == "" {
			continue
		}

		frame, err := DecodeBase64Frame(text)
		if err != nil {
			return nil, err
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan capture: %w", err)
	}
	return frames, nil
}
