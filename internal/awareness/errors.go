package awareness

import "errors"

var errNoIdleData = errors.New("awareness: no HIDIdleTime in ioreg output")
