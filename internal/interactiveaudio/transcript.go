package interactiveaudio

import (
	"fmt"
	"io"
	"strings"
)

const (
	ansiGrey  = "\033[90m"
	ansiBold  = "\033[1m"
	ansiReset = "\033[0m"
)

// Renderer renders live transcript and status events.
type Renderer struct {
	out    io.Writer
	status io.Writer
	tty    bool
	debug  bool

	pendingUser       UserUtterance
	pendingAgents     []AgentUtterance
	lastAgentSpeakers []string
	lastEmittedAgent  AgentUtterance
	ttsActive         bool
}

// NewRenderer creates a transcript renderer.
func NewRenderer(out, status io.Writer, tty bool) *Renderer {
	if out == nil {
		out = io.Discard
	}
	if status == nil {
		status = io.Discard
	}
	return &Renderer{
		out:    out,
		status: status,
		tty:    tty,
	}
}

// SetDebug enables debug/status event logging.
func (r *Renderer) SetDebug(debug bool) {
	r.debug = debug
}

// Handle renders one decoded interactive-audio event.
func (r *Renderer) Handle(event Event) error {
	switch e := event.(type) {
	case UserUtterance:
		return r.handleUser(e)
	case AgentUtterance:
		return r.handleAgent(e)
	case MicrophoneEvent:
		return r.handleMicrophone(e)
	case StatusMessage:
		return r.debugEvent("status", e.Text)
	case TTSEvent:
		return r.handleTTS(e)
	case SendAudioEvent:
		return r.debugEvent("audio", fmt.Sprintf("utterance=%s trigger=%d", e.UtteranceID, e.TriggerType))
	case PlaybackEvent:
		return r.debugEvent("playback", e.State())
	case UnknownEvent:
		return r.debugEvent("unknown", fmt.Sprintf("%d", e.Type))
	default:
		return nil
	}
}

// Finish flushes any buffered transcript text.
func (r *Renderer) Finish() error {
	if err := r.flushPendingUser(); err != nil {
		return err
	}
	if err := r.flushPendingAgents(); err != nil {
		return err
	}
	return nil
}

func (r *Renderer) handleUser(e UserUtterance) error {
	if e.Transcript == "" {
		e.Transcript = r.pendingUser.Transcript
	}

	if r.tty {
		if e.IsFinal {
			r.pendingUser = UserUtterance{}
			_, err := fmt.Fprintf(r.out, "You: %s\n", strings.TrimSpace(e.Transcript))
			return err
		}
		r.pendingUser = e
		return nil
	}

	r.pendingUser = e
	if e.IsFinal {
		return r.flushPendingUser()
	}
	return nil
}

func (r *Renderer) handleAgent(e AgentUtterance) error {
	if len(e.Speakers) == 0 {
		e.Speakers = r.lastAgentSpeakers
	}
	if len(e.Speakers) == 0 {
		e.Speakers = []string{"Host Speaker"}
	}
	r.lastAgentSpeakers = append([]string(nil), e.Speakers...)

	e.Transcript = strings.TrimSpace(e.Transcript)
	if e.Transcript == "" {
		if e.IsFinal {
			return r.flushPendingAgents()
		}
		return nil
	}

	if r.shouldSkipAgent(e) {
		if e.IsFinal {
			return r.flushPendingAgents()
		}
		return nil
	}
	if r.ttsActive {
		return r.emitAgent(e)
	}
	r.pendingAgents = append(r.pendingAgents, e)
	if e.IsFinal {
		return r.flushPendingAgents()
	}
	return nil
}

func (r *Renderer) handleTTS(e TTSEvent) error {
	if err := r.debugEvent("tts", fmt.Sprintf("utterance=%s segment=%d event=%d", e.UtteranceID, e.SegmentIdx, e.EventType)); err != nil {
		return err
	}
	switch e.EventType {
	case 1:
		r.ttsActive = true
		return r.flushPendingAgents()
	case 2:
		r.ttsActive = false
		return r.flushPendingAgents()
	default:
		return nil
	}
}

func (r *Renderer) shouldSkipAgent(e AgentUtterance) bool {
	if r.lastEmittedAgent.UtteranceID == e.UtteranceID && r.lastEmittedAgent.Transcript == e.Transcript {
		return true
	}
	if len(r.pendingAgents) == 0 {
		return false
	}
	last := r.pendingAgents[len(r.pendingAgents)-1]
	return last.UtteranceID == e.UtteranceID && last.Transcript == e.Transcript
}

func (r *Renderer) emitAgent(e AgentUtterance) error {
	r.lastEmittedAgent = e
	if r.tty {
		_, err := fmt.Fprintf(r.out, "%s%s%s: %s\n", ansiBold, e.Speakers[0], ansiReset, e.Transcript)
		return err
	}
	_, err := fmt.Fprintf(r.out, "[HOST] %s\n", e.Transcript)
	return err
}

func (r *Renderer) handleMicrophone(e MicrophoneEvent) error {
	if !r.tty {
		return nil
	}
	if e.StatusCode == 3 {
		return nil
	}
	_, err := fmt.Fprintf(r.status, "%s[user speaking]%s\n", ansiGrey, ansiReset)
	return err
}

func (r *Renderer) debugEvent(kind, text string) error {
	if !r.debug || !r.tty {
		return nil
	}
	if strings.TrimSpace(text) == "" {
		text = "(empty)"
	}
	_, err := fmt.Fprintf(r.status, "%s[%s] %s%s\n", ansiGrey, kind, text, ansiReset)
	return err
}

func (r *Renderer) flushPendingUser() error {
	if strings.TrimSpace(r.pendingUser.Transcript) == "" {
		r.pendingUser = UserUtterance{}
		return nil
	}
	_, err := fmt.Fprintf(r.out, "[YOU] %s\n", strings.TrimSpace(r.pendingUser.Transcript))
	r.pendingUser = UserUtterance{}
	return err
}

func (r *Renderer) flushPendingAgents() error {
	for _, agent := range r.pendingAgents {
		if strings.TrimSpace(agent.Transcript) == "" {
			continue
		}
		if err := r.emitAgent(agent); err != nil {
			return err
		}
	}
	r.pendingAgents = nil
	return nil
}
