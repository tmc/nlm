//go:build darwin

package interactiveaudio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/tmc/nlm/internal/auth"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"golang.org/x/term"
)

// Options controls an interactive-audio run.
type Options struct {
	Config                Config
	AudioOverviewID       string
	Debug                 bool
	Stdout                io.Writer
	Stderr                io.Writer
	TTY                   bool
	SignalerAuthorization string
	MicToggle             <-chan struct{}
	MicSetState           func(bool)
	MicClose              func()
}

type session struct {
	opts       Options
	notebookID string
	cookies    string
	rpcClient  *rpc.Client
	renderer   *Renderer
	backend    *Backend
	stderr     io.Writer
	outbound   outboundState

	controlMu sync.Mutex
	controlDC *webrtc.DataChannel

	playbackMu      sync.Mutex
	finalTTS        bool
	lastSignalAt    time.Time
	lastRemoteAudio time.Time
	stopSent        bool
	stopSentAt      time.Time
}

const (
	playbackCompletionEventType = 2
	playbackQuietWindow         = time.Second
	playbackAckTimeout          = 1500 * time.Millisecond
	playbackPollInterval        = 100 * time.Millisecond
)

type sessionMessage struct {
	frame Frame
	err   error
}

type signalerStarter interface {
	SetDebug(bool)
	StartInteractiveAudioChannel(context.Context, string) error
}

var newSignalerClient = func(cookies, authorization string) (signalerStarter, error) {
	return auth.NewSignalerClient(cookies, authorization)
}

var (
	isTerminalFD    = term.IsTerminal
	makeTerminalRaw = term.MakeRaw
	restoreTerminal = term.Restore
)

// Run starts an interactive-audio session.
func Run(ctx context.Context, authToken, cookies, notebookID string, opts Options) error {
	if strings.TrimSpace(authToken) == "" || strings.TrimSpace(cookies) == "" {
		return fmt.Errorf("interactive audio requires authentication")
	}
	if strings.TrimSpace(notebookID) == "" {
		return fmt.Errorf("missing notebook id")
	}
	opts.AudioOverviewID = strings.TrimSpace(opts.AudioOverviewID)
	if opts.AudioOverviewID == "" {
		return fmt.Errorf("interactive audio requires audio overview id")
	}

	opts.Config.Speaker = strings.TrimSpace(opts.Config.Speaker)
	opts.Config.Mic = strings.TrimSpace(opts.Config.Mic)
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.Config.Speaker != "" {
		return fmt.Errorf("speaker selection is not wired yet; use --transcript-only")
	}
	if opts.Config.Mic != "" {
		return fmt.Errorf("microphone selection is not wired yet; use --transcript-only")
	}
	if opts.Config.TranscriptOnly {
		opts.Config.NoMic = true
	}

	backend, err := New(opts.Config)
	if err != nil {
		return err
	}
	defer backend.Close()
	if err := backend.StartPlayback(); err != nil {
		return err
	}

	renderer := NewRenderer(opts.Stdout, opts.Stderr, opts.TTY)
	renderer.SetDebug(opts.Debug)
	defer renderer.Finish()

	s := &session{
		opts:       opts,
		notebookID: notebookID,
		cookies:    cookies,
		rpcClient:  newRPCClient(authToken, cookies, opts.Debug),
		renderer:   renderer,
		backend:    backend,
		stderr:     opts.Stderr,
	}

	return s.run(ctx)
}

func (s *session) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	fmt.Fprintln(s.stderr, "Connecting to interactive audio session...")

	s.startSignaler(ctx)

	token, err := fetchInteractivityToken(s.rpcClient, s.notebookID)
	if err != nil {
		return err
	}
	if token.BlockStatus != "" && token.BlockStatus != "NOT_BLOCKED" {
		return fmt.Errorf("interactive audio unavailable: %s", token.BlockStatus)
	}

	config := webrtc.Configuration{
		ICEServers: toWebRTCICEServers(token.ICEServers),
	}
	if strings.EqualFold(token.ICETransportPolicy, "relay") {
		config.ICETransportPolicy = webrtc.ICETransportPolicyRelay
	}

	pc, err := newNotebookLMPeerConnection(config)
	if err != nil {
		return err
	}
	defer pc.Close()
	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()

	audioTransceiver, err := pc.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendrecv},
	)
	if err != nil {
		return fmt.Errorf("add audio transceiver: %w", err)
	}

	var micSender *localAudioSender
	if !s.passivePlaybackEnabled() {
		localTrack, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeOpus,
				ClockRate:    uplinkSampleRate,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1",
				RTCPFeedback: []webrtc.RTCPFeedback{{Type: "transport-cc"}},
			},
			"audio",
			"interactive-audio",
		)
		if err != nil {
			return fmt.Errorf("create local audio track: %w", err)
		}
		if err := audioTransceiver.Sender().ReplaceTrack(localTrack); err != nil {
			return fmt.Errorf("attach local audio track: %w", err)
		}
		micSender, err = newLocalAudioSender(localTrack, s.sendMicInterruption, s.stderr, s.opts.Debug)
		if err != nil {
			return err
		}
	}

	events := make(chan sessionMessage, 128)
	connErrs := make(chan error, 8)
	connected := make(chan struct{})
	var connectOnce sync.Once

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			connectOnce.Do(func() { close(connected) })
		case webrtc.PeerConnectionStateFailed:
			sendSessionError(connErrs, fmt.Errorf("peer connection failed"))
		case webrtc.PeerConnectionStateClosed:
			sendSessionError(connErrs, fmt.Errorf("peer connection closed"))
		}
		if s.opts.Debug {
			fmt.Fprintf(s.stderr, "Peer connection state: %s\n", state.String())
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if err := s.handleRemoteTrack(track); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
			sendSessionError(connErrs, err)
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		s.attachDataChannel(ctx, dc, events, connErrs)
	})

	dc, err := pc.CreateDataChannel("webrtc-datachannel", nil)
	if err != nil {
		return fmt.Errorf("create data channel: %w", err)
	}
	s.attachDataChannel(ctx, dc, events, connErrs)

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	fmt.Fprintln(s.stderr, "[ice] gathering candidates (1s timeout)")
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	gatherCtx, cancelGather := context.WithTimeout(ctx, time.Second)
	defer cancelGather()
	select {
	case <-gatherComplete:
	case <-gatherCtx.Done():
	}

	local := pc.LocalDescription()
	if local == nil {
		return fmt.Errorf("local description is nil")
	}

	answer, err := exchangeSDP(s.rpcClient, s.notebookID, normalizeOfferForNotebookLM(local.SDP))
	if err != nil {
		return err
	}
	fmt.Fprintln(s.stderr, "[sdp] offer sent, answer received")
	if err := pc.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	select {
	case <-connected:
		fmt.Fprintln(s.stderr, "[connected]")
	case err := <-connErrs:
		fmt.Fprintln(s.stderr, "[disconnected]")
		return err
	case <-ctx.Done():
		fmt.Fprintln(s.stderr, "[disconnected]")
		return nil
	}

	if micSender != nil {
		if err := s.backend.StartCapture(func(samples []int16, sampleRate, channels int) error {
			err := micSender.HandlePCM16(samples, sampleRate, channels)
			if err != nil {
				sendSessionError(connErrs, err)
			}
			return err
		}); err != nil {
			return err
		}
		stopKeys, err := s.startMicControl(ctx, cancel, micSender, connErrs)
		if err != nil {
			return err
		}
		defer stopKeys()
	}

	ticker := time.NewTicker(playbackPollInterval)
	defer ticker.Stop()

	for {
		select {
		case msg := <-events:
			if msg.err != nil {
				return msg.err
			}
			if msg.frame.Event == nil {
				continue
			}
			done, err := s.handleEvent(ctx, msg.frame.Event)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		case <-ticker.C:
			done, err := s.pollPlaybackCompletion()
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		case err := <-connErrs:
			if err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintln(s.stderr, "[disconnected]")
				return err
			}
			fmt.Fprintln(s.stderr, "[disconnected]")
			return nil
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				fmt.Fprintln(s.stderr, "Interactive audio session timed out.")
			}
			fmt.Fprintln(s.stderr, "[disconnected]")
			return nil
		}
	}
}

func (s *session) startMicControl(
	ctx context.Context,
	cancel context.CancelFunc,
	sender *localAudioSender,
	connErrs chan<- error,
) (func(), error) {
	if sender == nil || s.opts.Config.NoMic || s.opts.Config.TranscriptOnly {
		return func() {}, nil
	}

	var cleanups []func()
	setEnabled := func(enabled bool, source string) {
		if sender.SetEnabled(enabled) != enabled {
			enabled = sender.Enabled()
		}
		if s.opts.MicSetState != nil {
			s.opts.MicSetState(enabled)
		}
		switch {
		case enabled && source == "app":
			fmt.Fprintln(s.stderr, "Mic: on. Speak now. Click the mic window or press 'm' again to mute.")
		case enabled:
			fmt.Fprintln(s.stderr, "Mic: on. Speak now. Press 'm' again to mute.")
		case source == "app":
			fmt.Fprintln(s.stderr, "Mic: off. Click the mic window or press 'm' to talk.")
		default:
			fmt.Fprintln(s.stderr, "Mic: off. Press 'm' to talk.")
		}
	}
	toggle := func(source string) {
		setEnabled(!sender.Enabled(), source)
	}

	if s.opts.MicClose != nil {
		cleanups = append(cleanups, s.opts.MicClose)
	}
	if s.opts.MicSetState != nil {
		s.opts.MicSetState(sender.Enabled())
	}
	if s.opts.MicToggle != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-s.opts.MicToggle:
					if !ok {
						return
					}
					toggle("app")
				}
			}
		}()
	}

	if !s.opts.TTY {
		if s.opts.MicToggle == nil {
			fmt.Fprintln(s.stderr, "Mic muted by default. Rerun in a TTY or use --mic-app to turn it on.")
		} else {
			fmt.Fprintln(s.stderr, "Mic muted by default. Use the mic window to turn it on or off.")
		}
		return func() {
			for i := len(cleanups) - 1; i >= 0; i-- {
				cleanups[i]()
			}
		}, nil
	}

	fd := int(os.Stdin.Fd())
	if !isTerminalFD(fd) {
		if s.opts.MicToggle == nil {
			fmt.Fprintln(s.stderr, "Mic muted by default. Rerun in a TTY or use --mic-app to turn it on.")
		} else {
			fmt.Fprintln(s.stderr, "Mic muted by default. Use the mic window to turn it on or off.")
		}
		return func() {
			for i := len(cleanups) - 1; i >= 0; i-- {
				cleanups[i]()
			}
		}, nil
	}

	state, err := makeTerminalRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("enable raw terminal mode: %w", err)
	}
	cleanups = append(cleanups, func() {
		_ = restoreTerminal(fd, state)
	})

	if s.opts.MicToggle != nil {
		fmt.Fprintln(s.stderr, "Mic muted by default. Press 'm' or use the mic window to toggle it on or off. Press Ctrl-C to quit.")
	} else {
		fmt.Fprintln(s.stderr, "Mic muted by default. Press 'm' to toggle it on or off. Press Ctrl-C to quit.")
	}

	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				sendSessionError(connErrs, fmt.Errorf("read mic toggle key: %w", err))
				return
			}
			if n == 0 {
				continue
			}
			switch buf[0] {
			case 3:
				cancel()
				return
			case 'm', 'M':
				toggle("tty")
			}
		}
	}()

	return func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}, nil
}

func (s *session) handleEvent(_ context.Context, event Event) (bool, error) {
	if err := s.renderer.Handle(event); err != nil {
		return false, err
	}
	if s.recordPassivePlaybackEvent(event) {
		return s.finishPlayback()
	}
	if !s.passivePlaybackObserved() {
		return false, nil
	}
	return false, nil
}

func (s *session) recordPassivePlaybackEvent(event Event) bool {
	if !s.passivePlaybackEnabled() {
		return false
	}
	now := time.Now()

	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()

	switch e := event.(type) {
	case TTSEvent:
		if e.EventType == playbackCompletionEventType {
			s.finalTTS = true
			s.lastSignalAt = now
		}
	case SendAudioEvent:
		if s.finalTTS {
			s.lastSignalAt = now
		}
	case PlaybackEvent:
		if s.finalTTS {
			s.lastSignalAt = now
			return s.stopSent
		}
	}
	return false
}

func (s *session) pollPlaybackCompletion() (bool, error) {
	if !s.passivePlaybackEnabled() {
		return false, nil
	}
	if s.shouldSendPlaybackStop(time.Now()) {
		if err := s.sendPlaybackStop(); err != nil {
			return false, err
		}
		return false, nil
	}
	if s.shouldFinishPlayback(time.Now()) {
		return s.finishPlayback()
	}
	return false, nil
}

func (s *session) passivePlaybackEnabled() bool {
	return s.opts.Config.NoMic || s.opts.Config.TranscriptOnly
}

func (s *session) sendMicInterruption() error {
	frames := s.outbound.interruptionFrames()
	if len(frames) == 0 {
		return nil
	}
	dc := s.controlChannel()
	if dc == nil {
		return fmt.Errorf("interactive audio control channel is unavailable")
	}
	if err := sendFrames(dc, frames); err != nil {
		return err
	}
	if s.opts.Debug {
		fmt.Fprintf(s.stderr, "Sent %d interruption data-channel frames\n", len(frames))
	}
	return nil
}

func (s *session) passivePlaybackObserved() bool {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	return s.finalTTS
}

func (s *session) shouldSendPlaybackStop(now time.Time) bool {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()

	if !s.finalTTS || s.stopSent {
		return false
	}
	if s.backend != nil && !s.backend.PlaybackIdle() {
		return false
	}
	last := s.lastSignalAt
	if s.lastRemoteAudio.After(last) {
		last = s.lastRemoteAudio
	}
	if last.IsZero() {
		last = now
	}
	return now.Sub(last) >= playbackQuietWindow
}

func (s *session) shouldFinishPlayback(now time.Time) bool {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()

	if !s.stopSent {
		return false
	}
	if s.backend != nil && !s.backend.PlaybackIdle() {
		return false
	}
	return now.Sub(s.stopSentAt) >= playbackAckTimeout
}

func (s *session) sendPlaybackStop() error {
	frames := s.outbound.completionFrames()
	if len(frames) == 0 {
		return nil
	}
	dc := s.controlChannel()
	if dc == nil {
		return nil
	}
	if err := sendFrames(dc, frames); err != nil {
		return err
	}
	s.playbackMu.Lock()
	s.stopSent = true
	s.stopSentAt = time.Now()
	s.playbackMu.Unlock()
	if s.opts.Debug {
		fmt.Fprintf(s.stderr, "Sent %d playback completion data-channel frames\n", len(frames))
	}
	return nil
}

func (s *session) finishPlayback() (bool, error) {
	fmt.Fprintln(s.stderr, "Playback complete.")
	return true, nil
}

func (s *session) markRemoteAudioActivity() {
	s.playbackMu.Lock()
	s.lastRemoteAudio = time.Now()
	s.playbackMu.Unlock()
}

func (s *session) controlChannel() *webrtc.DataChannel {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	return s.controlDC
}

func (s *session) setControlChannel(dc *webrtc.DataChannel) {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	if dc == nil {
		return
	}
	if s.controlDC == nil || dc.Label() == "data-channel" {
		s.controlDC = dc
	}
}

func (s *session) clearControlChannel(dc *webrtc.DataChannel) {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	if s.controlDC == dc {
		s.controlDC = nil
	}
}

func (s *session) startSignaler(ctx context.Context) {
	signaler, err := newSignalerClient(s.cookies, s.opts.SignalerAuthorization)
	if err != nil {
		if s.opts.Debug {
			fmt.Fprintf(s.stderr, "[signaler] unavailable: %v\n", err)
		}
		return
	}
	signaler.SetDebug(s.opts.Debug)
	if err := signaler.StartInteractiveAudioChannel(ctx, s.notebookID); err != nil {
		if s.opts.Debug {
			fmt.Fprintf(s.stderr, "[signaler] start failed: %v\n", err)
		}
		return
	}
	if s.opts.Debug {
		fmt.Fprintln(s.stderr, "[signaler] channel started")
	}
}

func (s *session) attachDataChannel(ctx context.Context, dc *webrtc.DataChannel, events chan<- sessionMessage, connErrs chan<- error) {
	dc.OnOpen(func() {
		if s.opts.Debug {
			fmt.Fprintf(s.stderr, "Data channel open: %s\n", dc.Label())
		}
		if dc.Label() != "data-channel" && dc.Label() != "webrtc-datachannel" {
			return
		}
		s.setControlChannel(dc)
		frames := s.outbound.startupFrames(s.opts.AudioOverviewID)
		if len(frames) == 0 {
			return
		}
		if err := sendFrames(dc, frames); err != nil {
			sendSessionError(connErrs, err)
			return
		}
		if s.opts.Debug {
			fmt.Fprintf(s.stderr, "Sent %d startup data-channel frames\n", len(frames))
		}
	})
	dc.OnClose(func() {
		s.clearControlChannel(dc)
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		frame, err := DecodeFrame(msg.Data)
		if err != nil {
			if s.opts.Debug {
				fmt.Fprintf(s.stderr, "Ignoring malformed data-channel payload: %v\n", err)
			}
			return
		}
		select {
		case events <- sessionMessage{frame: frame}:
		case <-ctx.Done():
		}
	})
}

func sendSessionError(ch chan<- error, err error) {
	select {
	case ch <- err:
	default:
	}
}
