package server

import (
	"fmt"
	"time"
)

func elapsedTime(activeNanos int64, startedAt *int64) string {
	active := time.Duration(activeNanos)
	if startedAt != nil {
		active += max(0, time.Since(time.Unix(0, *startedAt)))
	}
	seconds := int64(active / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}
