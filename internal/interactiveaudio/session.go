package interactiveaudio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
)

// Options controls an interactive-audio run.
type Options struct {
	Config Config
	Debug  bool
	Stdout io.Writer
	Stderr io.Writer
	TTY    bool
}

type session struct {
	opts       Options
	notebookID string
	rpcClient  *rpc.Client
	renderer   *Renderer
	backend    *Backend
	stderr     io.Writer
}

type sessionMessage struct {
	frame Frame
	err   error
}

// Run starts an interactive-audio session.
func Run(ctx context.Context, authToken, cookies, notebookID string, opts Options) error {
	if strings.TrimSpace(authToken) == "" || strings.TrimSpace(cookies) == "" {
		return fmt.Errorf("interactive audio requires authentication")
	}
	if strings.TrimSpace(notebookID) == "" {
		return fmt.Errorf("missing notebook id")
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
	if !opts.Config.TranscriptOnly {
		return fmt.Errorf("local audio playback is not wired yet; use --transcript-only")
	}
	opts.Config.NoMic = true

	backend, err := New(opts.Config)
	if err != nil {
		return err
	}
	defer backend.Close()

	renderer := NewRenderer(opts.Stdout, opts.Stderr, opts.TTY)
	renderer.SetDebug(opts.Debug)
	defer renderer.Finish()

	s := &session{
		opts:       opts,
		notebookID: notebookID,
		rpcClient:  newRPCClient(authToken, cookies, opts.Debug),
		renderer:   renderer,
		backend:    backend,
		stderr:     opts.Stderr,
	}

	return s.run(ctx)
}

func (s *session) run(ctx context.Context) error {
	fmt.Fprintln(s.stderr, "Connecting to interactive audio session...")

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

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}
	defer pc.Close()

	_, err = pc.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendrecv},
	)
	if err != nil {
		return fmt.Errorf("add audio transceiver: %w", err)
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
		if s.opts.Debug {
			fmt.Fprintf(s.stderr, "Ignoring remote %s track (%s) in transcript-only mode\n", track.Kind().String(), track.Codec().MimeType)
		}
		buf := make([]byte, 1500)
		for {
			if _, _, err := track.Read(buf); err != nil {
				return
			}
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		s.attachDataChannel(ctx, dc, events)
	})

	dc, err := pc.CreateDataChannel("data-channel", nil)
	if err != nil {
		return fmt.Errorf("create data channel: %w", err)
	}
	s.attachDataChannel(ctx, dc, events)

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

	answer, err := exchangeSDP(s.rpcClient, s.notebookID, local.SDP)
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

	for {
		select {
		case msg := <-events:
			if msg.err != nil {
				return msg.err
			}
			if msg.frame.Event == nil {
				continue
			}
			if err := s.renderer.Handle(msg.frame.Event); err != nil {
				return err
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

func (s *session) attachDataChannel(ctx context.Context, dc *webrtc.DataChannel, events chan<- sessionMessage) {
	dc.OnOpen(func() {
		if s.opts.Debug {
			fmt.Fprintf(s.stderr, "Data channel open: %s\n", dc.Label())
		}
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
