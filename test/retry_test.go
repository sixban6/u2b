package test

import (
	"errors"
	"testing"

	"github.com/sixban6/u2b/downloader"
)

func TestNeedsBrowserAuthRetry(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "bot check message",
			err:      errors.New("ERROR: Sign in to confirm you're not a bot"),
			expected: true,
		},
		{
			name:     "cookies hint message",
			err:      errors.New("Use --cookies-from-browser or --cookies"),
			expected: true,
		},
		{
			name:     "other message",
			err:      errors.New("network timeout"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := downloader.NeedsBrowserAuthRetry(tt.err)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}
