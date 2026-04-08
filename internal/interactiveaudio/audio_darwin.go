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
	"github.com/tmc/apple/objc"
)

// Config describes the requested local-audio mode.
type Config struct {
	TranscriptOnly bool
	NoMic          bool
	Speaker        string
	Mic            string
}

const (
	playbackQueueDepth       = 96
	playbackStartBuffers     = 6
	playbackStartMaxDelay    = 150 * time.Millisecond
	playbackPrepareFrameRate = 10
)

// Backend is the Darwin audio backend.
type Backend struct {
	cfg        Config
	engine     avfaudio.AVAudioEngine
	player     avfaudio.AVAudioPlayerNode
	input      avfaudio.AVAudioInputNode
	graphReady bool
	tapActive  bool
	tapBlock   objc.Block

	mu               sync.Mutex
	format           avfaudio.AVAudioFormat
	playbackStarted  bool
	playbackRate     int
	playbackChans    int
	playbackSlots    chan struct{}
	playbackPrimedAt time.Time
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
	if node := b.engine.InputNode(); node.GetID() != 0 {
		b.input = avfaudio.AVAudioInputNodeFromID(node.GetID())
	}
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
	if b.tapActive && b.input.ID != 0 {
		b.input.RemoveTapOnBus(0)
		b.tapActive = false
	}
	if b.tapBlock != 0 {
		b.tapBlock.Release()
		b.tapBlock = 0
	}
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
	b.playbackSlots <- struct{}{}

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
			b.mu.Lock()
			if b.playbackPending() == 0 {
				b.playbackPrimedAt = time.Time{}
			}
			b.mu.Unlock()
		},
	)
	if !b.player.Playing() {
		if b.playbackPrimedAt.IsZero() {
			b.playbackPrimedAt = time.Now()
		}
		if !shouldStartPlayback(b.playbackPending(), b.playbackPrimedAt, time.Now()) {
			return nil
		}
		b.player.Play()
		b.playbackPrimedAt = time.Time{}
	}
	return nil
}

// StartCapture begins microphone capture and forwards PCM buffers to handler.
func (b *Backend) StartCapture(handler captureHandler) error {
	if b == nil {
		return fmt.Errorf("interactive audio backend is nil")
	}
	if b.cfg.TranscriptOnly || b.cfg.NoMic {
		return nil
	}
	if !b.SupportsCapture() {
		return fmt.Errorf("interactive audio microphone graph is unavailable")
	}
	if handler == nil {
		return fmt.Errorf("interactive audio microphone capture requires a handler")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.input.ID == 0 {
		if node := b.engine.InputNode(); node.GetID() != 0 {
			b.input = avfaudio.AVAudioInputNodeFromID(node.GetID())
		}
	}
	if b.input.ID == 0 {
		return fmt.Errorf("interactive audio microphone input node is unavailable")
	}
	if b.tapActive {
		return nil
	}
	b.input.RemoveTapOnBus(0)
	tapBlock := objc.NewBlock(func(_ objc.Block, bufferID objc.ID, _ objc.ID) {
		buffer := avfaudio.AVAudioPCMBufferFromID(bufferID)
		samples, sampleRate, channels, err := decodeInputPCMBuffer(buffer)
		if err != nil || len(samples) == 0 {
			return
		}
		_ = handler(samples, sampleRate, channels)
	})
	objc.Send[objc.ID](
		b.input.ID,
		objc.Sel("installTapOnBus:bufferSize:format:block:"),
		avfaudio.AVAudioNodeBus(0),
		avfaudio.AVAudioFrameCount(uplinkFrameSamples),
		objc.ID(0),
		unsafe.Pointer(tapBlock),
	)
	b.tapBlock = tapBlock
	b.tapActive = true
	if !b.engine.Running() {
		ok, err := b.engine.StartAndReturnError()
		if err != nil {
			b.input.RemoveTapOnBus(0)
			b.tapActive = false
			if b.tapBlock != 0 {
				b.tapBlock.Release()
				b.tapBlock = 0
			}
			return fmt.Errorf("start interactive audio engine for capture: %w", err)
		}
		if !ok {
			b.input.RemoveTapOnBus(0)
			b.tapActive = false
			if b.tapBlock != 0 {
				b.tapBlock.Release()
				b.tapBlock = 0
			}
			return fmt.Errorf("start interactive audio engine for capture")
		}
	}
	return nil
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
	b.player.PrepareWithFrameCount(avfaudio.AVAudioFrameCount(sampleRate / playbackPrepareFrameRate))
	b.engine.Prepare()

	ok, err := b.engine.StartAndReturnError()
	if err != nil {
		return fmt.Errorf("start interactive audio engine: %w", err)
	}
	if !ok {
		return fmt.Errorf("start interactive audio engine")
	}
	b.format = format
	b.playbackRate = sampleRate
	b.playbackChans = channels
	b.playbackStarted = true
	return nil
}

func shouldStartPlayback(queued int, primedAt, now time.Time) bool {
	if queued >= playbackStartBuffers {
		return true
	}
	if queued <= 0 || primedAt.IsZero() {
		return false
	}
	return now.Sub(primedAt) >= playbackStartMaxDelay
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

func decodeInputPCMBuffer(buffer avfaudio.AVAudioPCMBuffer) ([]int16, int, int, error) {
	format := avfaudio.AVAudioFormatFromID(buffer.Format().GetID())
	if format.ID == 0 {
		return nil, 0, 0, fmt.Errorf("interactive audio microphone buffer format is unavailable")
	}
	frameCount := int(buffer.FrameLength())
	channels := int(format.ChannelCount())
	if frameCount == 0 || channels == 0 {
		return nil, int(format.SampleRate()), channels, nil
	}

	switch format.CommonFormat() {
	case avfaudio.AVAudioPCMFormatFloat32:
		samples, err := float32PCMBufferSamples(buffer, frameCount, channels, format.Interleaved())
		return samples, int(format.SampleRate()), channels, err
	case avfaudio.AVAudioPCMFormatInt16:
		samples, err := int16PCMBufferSamples(buffer, frameCount, channels, format.Interleaved())
		return samples, int(format.SampleRate()), channels, err
	case avfaudio.AVAudioPCMFormatInt32:
		samples, err := int32PCMBufferSamples(buffer, frameCount, channels, format.Interleaved())
		return samples, int(format.SampleRate()), channels, err
	default:
		return nil, 0, 0, fmt.Errorf("interactive audio microphone format %s is unsupported", format.CommonFormat())
	}
}

func int16PCMBufferSamples(buffer avfaudio.AVAudioPCMBuffer, frames, channels int, interleaved bool) ([]int16, error) {
	channelData := buffer.Int16ChannelData()
	if channelData == nil {
		return nil, fmt.Errorf("interactive audio microphone buffer returned no int16 data")
	}
	ptrs := unsafe.Slice((**int16)(channelData), channels)
	if len(ptrs) == 0 || ptrs[0] == nil {
		return nil, fmt.Errorf("interactive audio microphone buffer returned empty int16 data")
	}
	if interleaved {
		return append([]int16(nil), unsafe.Slice(ptrs[0], frames*channels)...), nil
	}
	samples := make([]int16, 0, frames*channels)
	for frame := 0; frame < frames; frame++ {
		for ch := 0; ch < channels; ch++ {
			if ptrs[ch] == nil {
				return nil, fmt.Errorf("interactive audio microphone buffer returned empty int16 channel %d", ch)
			}
			channel := unsafe.Slice(ptrs[ch], frames)
			samples = append(samples, channel[frame])
		}
	}
	return samples, nil
}

func float32PCMBufferSamples(buffer avfaudio.AVAudioPCMBuffer, frames, channels int, interleaved bool) ([]int16, error) {
	channelData := buffer.FloatChannelData()
	if channelData == nil {
		return nil, fmt.Errorf("interactive audio microphone buffer returned no float32 data")
	}
	ptrs := unsafe.Slice((**float32)(channelData), channels)
	if len(ptrs) == 0 || ptrs[0] == nil {
		return nil, fmt.Errorf("interactive audio microphone buffer returned empty float32 data")
	}
	if interleaved {
		raw := unsafe.Slice(ptrs[0], frames*channels)
		samples := make([]int16, len(raw))
		for i, sample := range raw {
			samples[i] = clampPCM16(float64(sample) * maxPCM16)
		}
		return samples, nil
	}
	samples := make([]int16, 0, frames*channels)
	for frame := 0; frame < frames; frame++ {
		for ch := 0; ch < channels; ch++ {
			if ptrs[ch] == nil {
				return nil, fmt.Errorf("interactive audio microphone buffer returned empty float32 channel %d", ch)
			}
			channel := unsafe.Slice(ptrs[ch], frames)
			samples = append(samples, clampPCM16(float64(channel[frame])*maxPCM16))
		}
	}
	return samples, nil
}

func int32PCMBufferSamples(buffer avfaudio.AVAudioPCMBuffer, frames, channels int, interleaved bool) ([]int16, error) {
	channelData := buffer.Int32ChannelData()
	if channelData == nil {
		return nil, fmt.Errorf("interactive audio microphone buffer returned no int32 data")
	}
	ptrs := unsafe.Slice((**int32)(channelData), channels)
	if len(ptrs) == 0 || ptrs[0] == nil {
		return nil, fmt.Errorf("interactive audio microphone buffer returned empty int32 data")
	}
	if interleaved {
		raw := unsafe.Slice(ptrs[0], frames*channels)
		samples := make([]int16, len(raw))
		for i, sample := range raw {
			samples[i] = clampPCM16(float64(sample) / 65536.0)
		}
		return samples, nil
	}
	samples := make([]int16, 0, frames*channels)
	for frame := 0; frame < frames; frame++ {
		for ch := 0; ch < channels; ch++ {
			if ptrs[ch] == nil {
				return nil, fmt.Errorf("interactive audio microphone buffer returned empty int32 channel %d", ch)
			}
			channel := unsafe.Slice(ptrs[ch], frames)
			samples = append(samples, clampPCM16(float64(channel[frame])/65536.0))
		}
	}
	return samples, nil
}
