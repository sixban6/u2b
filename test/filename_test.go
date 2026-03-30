package test

import (
	"testing"

	u2bcmd "git.sr.ht/~sixban/u2b_download/cmd"
)

func TestBuildOutputTemplateForFilename(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "simple filename",
			input:       "Star Sky",
			expected:    "Star Sky.%(ext)s",
			expectError: false,
		},
		{
			name:        "filename with extension",
			input:       "Star Sky.mp3",
			expected:    "Star Sky.%(ext)s",
			expectError: false,
		},
		{
			name:        "filename with invalid characters",
			input:       `Star/Sky:Live?`,
			expected:    "Star_Sky_Live_.%(ext)s",
			expectError: false,
		},
		{
			name:        "empty filename",
			input:       "   ",
			expectError: true,
		},
		{
			name:        "only extension",
			input:       ".mp3",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := u2bcmd.BuildOutputTemplateForFilename(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
