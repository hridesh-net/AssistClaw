//go:build darwin

package awareness

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

var hidIdleRe = regexp.MustCompile(`"HIDIdleTime"\s*=\s*(\d+)`)

// idleSeconds reads the HID idle time (seconds since last keyboard/mouse input).
func idleSeconds(ctx context.Context) (float64, error) {
	out, err := exec.CommandContext(ctx, "/usr/sbin/ioreg", "-c", "IOHIDSystem", "-d", "4").Output()
	if err != nil {
		return 0, err
	}
	m := hidIdleRe.FindSubmatch(out)
	if m == nil {
		return 0, errNoIdleData
	}
	ns, err := strconv.ParseUint(string(m[1]), 10, 64)
	if err != nil {
		return 0, err
	}
	return float64(ns) / 1e9, nil
}

// StartIdlePoller updates "user.activity" every interval based on input idle
// time: at the machine, idle, or away. Best-effort — errors disable the signal
// rather than spamming logs.
func StartIdlePoller(ctx context.Context, store *Store, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			secs, err := idleSeconds(pollCtx)
			cancel()
			if err == nil {
				idle := time.Duration(secs) * time.Second
				var state string
				switch {
				case idle < 3*time.Minute:
					state = "active at the computer"
				case idle < 20*time.Minute:
					state = "stepped away briefly (idle " + idle.Round(time.Minute).String() + ")"
				default:
					state = "away from the computer (idle " + idle.Round(time.Minute).String() + ")"
				}
				store.Set("user.activity", state, 2*interval+time.Minute)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}
