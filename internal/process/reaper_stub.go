//go:build !linux

package process

import (
	"context"
)

// Init is a no-op on non-Linux platforms.
func Init() error {
	return nil
}

// RunReaper is a no-op on non-Linux platforms.
func RunReaper(ctx context.Context) {
}
