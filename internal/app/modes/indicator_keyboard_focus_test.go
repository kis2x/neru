package modes

import (
	"context"
	"testing"

	configpkg "github.com/y3owk1n/neru/internal/config"
	"github.com/y3owk1n/neru/internal/domain"
	"github.com/y3owk1n/neru/internal/domain/state"
	portmocks "github.com/y3owk1n/neru/internal/ports/mocks"
)

// A writable uinput scroll device says nothing about keyboard capture. When
// evdev cannot read a keyboard, the Wayland fallback needs exclusive focus for
// the entire mode, including indicator restarts during mode transitions.
func TestIndicatorPolling_PreservesEventTapKeyboardOwnership(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name               string
		allowPassthrough   bool
		captureBeforeStart bool
		captureAfterStart  bool
		captureBeforeStop  bool
	}{
		{
			name:               "fallback_before_indicators",
			allowPassthrough:   true,
			captureBeforeStart: true,
			captureAfterStart:  true,
			captureBeforeStop:  true,
		},
		{
			name:              "fallback_after_indicators",
			allowPassthrough:  true,
			captureAfterStart: true,
			captureBeforeStop: true,
		},
		{
			name:               "fallback_released_before_indicator_cleanup",
			allowPassthrough:   true,
			captureBeforeStart: true,
			captureAfterStart:  true,
		},
		{
			name: "evdev_keeps_overlay_passive",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			overlay := &portmocks.MockOverlayPort{}
			handler := newHandlerWithState(handlerState{
				ctx:      context.Background(),
				appState: state.NewAppState(),
				config: &configpkg.Config{
					// Sticky modifiers alone enable polling in every mode;
					// none of the mode indicators needs to be enabled.
					StickyModifiers: configpkg.StickyModifiersConfig{Enabled: true},
				},
				overlayPort: overlay,
				eventTap:    &passthroughTap{allow: test.allowPassthrough},
			})

			// Exercise both orderings without sleeps. The handler lock is the
			// same one held across real activation and mode-transition cleanup.
			handler.mu.Lock()
			defer handler.mu.Unlock()
			defer handler.stopIndicatorPolling()

			overlay.SetKeyboardCaptureEnabled(test.captureBeforeStart)

			for cycle, mode := range []domain.Mode{
				domain.ModeScroll,
				domain.ModeGrid,
				domain.ModeRecursiveGrid,
				domain.ModeHints,
				domain.ModeScroll,
			} {
				handler.appState.SetMode(mode)

				beforeStart := overlay.KeyboardCaptureEnabled()

				handler.startIndicatorPolling(mode)

				if handler.indicatorTicker == nil {
					t.Fatal("indicator polling did not start")
				}

				if got := overlay.KeyboardCaptureEnabled(); got != beforeStart {
					t.Errorf("cycle %d: indicator startup changed keyboard capture to %v, want %v",
						cycle, got, beforeStart)
				}

				// The event tap completes capture (possibly after polling
				// started), then either hands it to the next mode or exits.
				overlay.SetKeyboardCaptureEnabled(test.captureAfterStart)
				overlay.SetKeyboardCaptureEnabled(test.captureBeforeStop)
				handler.stopIndicatorPolling()

				if got := overlay.KeyboardCaptureEnabled(); got != test.captureBeforeStop {
					t.Errorf("cycle %d: indicator cleanup changed keyboard capture to %v, want %v",
						cycle, got, test.captureBeforeStop)
				}

				if handler.indicatorTicker != nil || handler.indicatorDoneCh != nil {
					t.Fatal("indicator polling did not stop")
				}
			}
		})
	}
}
