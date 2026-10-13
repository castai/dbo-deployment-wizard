package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskPassword(t *testing.T) {
	tests := map[string]struct {
		password string
		want     int
	}{
		"short password shows its length": {password: "hunter2", want: 7},
		"long password capped":            {password: strings.Repeat("x", 40), want: maskedPasswordMax},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(strings.Repeat("•", tt.want), maskPassword(tt.password))
		})
	}
}
