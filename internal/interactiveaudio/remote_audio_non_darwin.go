//go:build !darwin

package interactiveaudio

import (
	"fmt"

	"github.com/pion/webrtc/v4"
)

func (s *session) handleRemoteTrack(track *webrtc.TrackRemote) error {
	if s.opts.Debug {
		fmt.Fprintf(s.stderr, "Ignoring remote %s track (%s): local playback requires darwin\n", track.Kind().String(), track.Codec().MimeType)
	}
	buf := make([]byte, 1500)
	for {
		if _, _, err := track.Read(buf); err != nil {
			return err
		}
	}
}
