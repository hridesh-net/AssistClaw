//go:build !darwin

package awareness

import (
	"context"
	"time"
)

// StartIdlePoller is a no-op on platforms without an idle-time probe yet.
// Linux support (xprintidle / logind IdleHint) can land here later.
func StartIdlePoller(ctx context.Context, store *Store, interval time.Duration) {
	_ = ctx
	_ = store
	_ = interval
}
