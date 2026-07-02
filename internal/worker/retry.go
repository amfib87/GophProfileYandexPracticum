package worker

import (
	"time"
)

func WithRetry(fn func() error, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		// Экспоненциальная задержка
		time.Sleep(time.Duration(1<<i) * time.Second)
	}
	return lastErr
}
