package testhooks

import (
	"context"
	"os"
	"time"
)

func ApplyFileGate(ctx context.Context, markerFile, releaseFile string) error {
	if markerFile == "" && releaseFile == "" {
		return nil
	}

	if markerFile != "" {
		if err := os.WriteFile(markerFile, []byte("ready"), 0o644); err != nil {
			return err
		}
	}
	if releaseFile == "" {
		return nil
	}

	return WaitForReleaseFile(ctx, releaseFile)
}

func WaitForReleaseFile(ctx context.Context, releaseFile string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if _, err := os.Stat(releaseFile); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
