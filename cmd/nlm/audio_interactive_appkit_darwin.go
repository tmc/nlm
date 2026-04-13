//go:build darwin

package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/tmc/apple/appkit"
	"github.com/tmc/apple/corefoundation"
	"github.com/tmc/apple/dispatch"
	"github.com/tmc/apple/foundation"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

type interactiveAudioMicWindow struct {
	app    appkit.NSApplication
	window appkit.NSWindow
	status appkit.NSTextField
	button appkit.NSButton

	toggleCh  chan struct{}
	closeOnce sync.Once
}

func lockInteractiveAudioAppThreadIfNeeded(args []string) {
	var audioInteractive bool
	for _, arg := range args {
		if arg == "audio-interactive" {
			audioInteractive = true
			continue
		}
		if audioInteractive && (arg == "--mic-app" || strings.HasPrefix(arg, "--mic-app=")) {
			runtime.LockOSThread()
			return
		}
	}
}

func runInteractiveAudioWithMicApp(client *api.Client, notebookID string, opts interactiveAudioOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	controller := newInteractiveAudioMicWindow()
	resultCh := make(chan error, 1)
	appkit.RunApp(func(app appkit.NSApplication, _ appkit.NSApplicationDelegateObject) {
		app.SetActivationPolicy(appkit.NSApplicationActivationPolicyAccessory)
		controller.app = app
		controller.build()
		go func() {
			err := runInteractiveAudioWithControllerContext(ctx, client, notebookID, opts, controller)
			resultCh <- err
			dispatch.MainQueue().Async(func() {
				app.Terminate(app)
			})
		}()
	})

	stop()
	err, ok := <-resultCh
	if !ok {
		return nil
	}
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func newInteractiveAudioMicWindow() *interactiveAudioMicWindow {
	return &interactiveAudioMicWindow{toggleCh: make(chan struct{}, 4)}
}

func (w *interactiveAudioMicWindow) ToggleEvents() <-chan struct{} {
	return w.toggleCh
}

func (w *interactiveAudioMicWindow) SetEnabled(enabled bool) {
	dispatch.MainQueue().Async(func() {
		if w.status.ID == 0 || w.button.ID == 0 {
			return
		}
		if enabled {
			w.status.SetStringValue("Mic is live")
			w.button.SetTitle("Mute Mic")
			return
		}
		w.status.SetStringValue("Mic is muted")
		w.button.SetTitle("Turn Mic On")
	})
}

func (w *interactiveAudioMicWindow) Close() {
	w.closeOnce.Do(func() {
		close(w.toggleCh)
		dispatch.MainQueue().Async(func() {
			if w.window.ID != 0 {
				w.window.Close()
			}
		})
	})
}

func (w *interactiveAudioMicWindow) build() {
	window := appkit.GetNSWindowClass().Alloc().InitWithContentRectStyleMaskBackingDefer(
		corefoundation.CGRect{
			Origin: corefoundation.CGPoint{X: 220, Y: 220},
			Size:   corefoundation.CGSize{Width: 340, Height: 170},
		},
		appkit.NSWindowStyleMaskTitled|appkit.NSWindowStyleMaskClosable,
		appkit.NSBackingStoreBuffered,
		false,
	)
	window.SetTitle("NotebookLM Mic")
	window.Center()

	status := appkit.NewTextFieldLabelWithString("Mic is muted")
	status.SetFont(appkit.GetNSFontClass().SystemFontOfSize(18))
	status.SetAlignment(appkit.NSTextAlignmentCenter)

	hint := appkit.NewTextFieldLabelWithString("Use this window or press 'm' in the terminal.")
	hint.SetFont(appkit.GetNSFontClass().SystemFontOfSize(12))
	hint.SetAlignment(appkit.NSTextAlignmentCenter)
	hint.SetTextColor(appkit.GetNSColorClass().SecondaryLabelColor())

	button := appkit.NewButtonWithTitleTargetAction("Turn Mic On", nil, 0)
	button.SetBezelStyle(appkit.NSBezelStyleRounded)
	button.SetActionHandler(func() {
		select {
		case w.toggleCh <- struct{}{}:
		default:
		}
	})

	stack := appkit.GetNSStackViewClass().Alloc().Init()
	stack.SetOrientation(appkit.NSUserInterfaceLayoutOrientationVertical)
	stack.SetAlignment(appkit.NSLayoutAttributeCenterX)
	stack.SetSpacing(14)
	stack.SetEdgeInsets(foundation.NSEdgeInsets{Top: 20, Left: 20, Bottom: 20, Right: 20})
	stack.AddArrangedSubview(status)
	stack.AddArrangedSubview(hint)
	stack.AddArrangedSubview(button)
	stack.SetTranslatesAutoresizingMaskIntoConstraints(false)

	contentView := window.ContentView().(appkit.NSView)
	contentView.AddSubview(stack)
	stack.LeadingAnchor().ConstraintEqualToAnchor(contentView.LeadingAnchor()).SetActive(true)
	stack.TrailingAnchor().ConstraintEqualToAnchor(contentView.TrailingAnchor()).SetActive(true)
	stack.TopAnchor().ConstraintEqualToAnchor(contentView.TopAnchor()).SetActive(true)
	stack.BottomAnchor().ConstraintEqualToAnchor(contentView.BottomAnchor()).SetActive(true)

	w.window = window
	w.status = status
	w.button = button

	window.MakeKeyAndOrderFront(nil)
	w.app.Activate()
	w.SetEnabled(false)
}
