//go:build darwin

package interactiveaudio

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/tmc/apple/avfaudio"
)

// Config describes the requested local-audio mode.
type Config struct {
	TranscriptOnly bool
	NoMic          bool
	Speaker        string
	Mic            string
}

const playbackQueueDepth = 32

// Backend is the Darwin audio backend.
type Backend struct {
	cfg        Config
	engine     avfaudio.AVAudioEngine
	player     avfaudio.AVAudioPlayerNode
	input      avfaudio.AVAudioInputNode
	graphReady bool

	mu              sync.Mutex
	format          avfaudio.AVAudioFormat
	playbackStarted bool
	playbackRate    int
	playbackChans   int
	playbackSlots   chan struct{}
}

// New creates the Darwin backend.
func New(cfg Config) (*Backend, error) {
	cfg.Speaker = strings.TrimSpace(cfg.Speaker)
	cfg.Mic = strings.TrimSpace(cfg.Mic)

	b := &Backend{cfg: cfg}
	if cfg.TranscriptOnly {
		return b, nil
	}

	// Construct the AVFAudio objects now so the backend shape is explicit and
	// compile-time checked against the local apple bindings.
	b.engine = avfaudio.NewAVAudioEngine()
	b.player = avfaudio.NewAVAudioPlayerNode()
	b.input = avfaudio.NewAVAudioInputNode()
	b.graphReady = true
	return b, nil
}

// Close releases local AVFAudio resources.
func (b *Backend) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.player.ID != 0 && b.player.Playing() {
		b.player.Stop()
	}
	if b.engine.ID != 0 {
		b.engine.Stop()
		b.engine.Reset()
	}
	return nil
}

// TranscriptOnly reports whether local audio is disabled.
func (b *Backend) TranscriptOnly() bool {
	return b != nil && b.cfg.TranscriptOnly
}

// SupportsPlayback reports whether a local playback graph is available.
func (b *Backend) SupportsPlayback() bool {
	return b != nil && b.graphReady && !b.cfg.TranscriptOnly
}

// SupportsCapture reports whether a local microphone graph is available.
func (b *Backend) SupportsCapture() bool {
	return b != nil && b.graphReady && !b.cfg.TranscriptOnly && !b.cfg.NoMic
}

// StartPlayback reserves the AVAudioEngine/AVAudioPlayerNode path.
func (b *Backend) StartPlayback() error {
	if b == nil {
		return fmt.Errorf("interactive audio backend is nil")
	}
	if b.cfg.TranscriptOnly {
		return nil
	}
	if !b.graphReady {
		return fmt.Errorf("interactive audio playback graph is unavailable")
	}
	if b.playbackSlots == nil {
		b.playbackSlots = make(chan struct{}, playbackQueueDepth)
	}
	return nil
}

// WritePCM16 schedules interleaved 16-bit PCM for playback.
func (b *Backend) WritePCM16(samples []int16, sampleRate, channels int) error {
	if b == nil {
		return fmt.Errorf("interactive audio backend is nil")
	}
	if b.cfg.TranscriptOnly || len(samples) == 0 {
		return nil
	}
	if !b.graphReady {
		return fmt.Errorf("interactive audio playback graph is unavailable")
	}
	if channels <= 0 {
		return fmt.Errorf("interactive audio playback requires positive channel count")
	}
	if len(samples)%channels != 0 {
		return fmt.Errorf("interactive audio playback requires interleaved pcm aligned to channels")
	}
	if b.playbackSlots == nil {
		b.playbackSlots = make(chan struct{}, playbackQueueDepth)
	}
	select {
	case b.playbackSlots <- struct{}{}:
	default:
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.ensurePlaybackGraph(sampleRate, channels); err != nil {
		<-b.playbackSlots
		return err
	}

	buffer, err := b.newPlaybackBuffer(samples, channels)
	if err != nil {
		<-b.playbackSlots
		return err
	}
	b.player.ScheduleBufferCompletionCallbackTypeCompletionHandler(
		buffer,
		avfaudio.AVAudioPlayerNodeCompletionDataPlayedBack,
		func() {
			_ = buffer
			select {
			case <-b.playbackSlots:
			default:
			}
		},
	)
	if !b.player.Playing() {
		b.player.Play()
	}
	return nil
}

// StartCapture reserves the AVAudioEngine/AVAudioInputNode path.
func (b *Backend) StartCapture() error {
	if b == nil {
		return fmt.Errorf("interactive audio backend is nil")
	}
	if b.cfg.TranscriptOnly || b.cfg.NoMic {
		return nil
	}
	if !b.SupportsCapture() {
		return fmt.Errorf("interactive audio microphone graph is unavailable")
	}
	return fmt.Errorf("interactive audio microphone capture graph is ready on darwin, but outbound audio encode is not wired yet")
}

// WaitPlaybackIdle waits until queued playback has drained and stayed idle for
// at least settle.
func (b *Backend) WaitPlaybackIdle(ctx context.Context, settle time.Duration) error {
	if b == nil || b.cfg.TranscriptOnly {
		return nil
	}
	if settle <= 0 {
		settle = 250 * time.Millisecond
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	var idleSince time.Time
	for {
		if b.playbackPending() == 0 {
			if idleSince.IsZero() {
				idleSince = time.Now()
			}
			if time.Since(idleSince) >= settle {
				return nil
			}
		} else {
			idleSince = time.Time{}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// PlaybackIdle reports whether the local playback queue is empty.
func (b *Backend) PlaybackIdle() bool {
	return b.playbackPending() == 0
}

func (b *Backend) ensurePlaybackGraph(sampleRate, channels int) error {
	if sampleRate <= 0 {
		return fmt.Errorf("interactive audio playback requires positive sample rate")
	}
	if channels != 1 && channels != 2 {
		return fmt.Errorf("interactive audio playback requires mono or stereo pcm")
	}
	if b.playbackStarted {
		if b.playbackRate != sampleRate || b.playbackChans != channels {
			return fmt.Errorf(
				"interactive audio playback format changed from %d Hz/%d ch to %d Hz/%d ch",
				b.playbackRate,
				b.playbackChans,
				sampleRate,
				channels,
			)
		}
		return nil
	}

	format := avfaudio.NewAudioFormatWithCommonFormatSampleRateChannelsInterleaved(
		avfaudio.AVAudioPCMFormatInt16,
		float64(sampleRate),
		avfaudio.AVAudioChannelCount(channels),
		true,
	)
	if format.ID == 0 {
		return fmt.Errorf("create interactive audio playback format")
	}

	b.engine.AttachNode(b.player)
	b.engine.ConnectToFormat(b.player, b.engine.MainMixerNode(), format)
	b.player.PrepareWithFrameCount(avfaudio.AVAudioFrameCount(sampleRate / 50))
	b.engine.Prepare()

	ok, err := b.engine.StartAndReturnError()
	if err != nil {
		return fmt.Errorf("start interactive audio engine: %w", err)
	}
	if !ok {
		return fmt.Errorf("start interactive audio engine")
	}
	b.player.Play()

	b.format = format
	b.playbackRate = sampleRate
	b.playbackChans = channels
	b.playbackStarted = true
	return nil
}

func (b *Backend) playbackPending() int {
	if b == nil || b.playbackSlots == nil {
		return 0
	}
	return len(b.playbackSlots)
}

func (b *Backend) newPlaybackBuffer(samples []int16, channels int) (avfaudio.AVAudioPCMBuffer, error) {
	frameCount := len(samples) / channels
	buffer := avfaudio.NewAudioPCMBufferWithPCMFormatFrameCapacity(
		b.format,
		avfaudio.AVAudioFrameCount(frameCount),
	)
	if buffer.ID == 0 {
		return avfaudio.AVAudioPCMBuffer{}, fmt.Errorf("create interactive audio playback buffer")
	}

	channelData := buffer.Int16ChannelData()
	if channelData == nil {
		return avfaudio.AVAudioPCMBuffer{}, fmt.Errorf("interactive audio playback buffer has no int16 channel data")
	}
	channelPtrs := unsafe.Slice((**int16)(channelData), channels)
	if len(channelPtrs) == 0 || channelPtrs[0] == nil {
		return avfaudio.AVAudioPCMBuffer{}, fmt.Errorf("interactive audio playback buffer returned empty channel data")
	}
	dst := unsafe.Slice(channelPtrs[0], len(samples))
	copy(dst, samples)
	buffer.SetFrameLength(avfaudio.AVAudioFrameCount(frameCount))
	return buffer, nil
}
