package tui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/stretchr/testify/require"
)

func TestConfirmTick_SpinsOnlyWhileLoading(t *testing.T) {
	c := newConfirmModel(&stubBackend{}, huh.ThemeFunc(huh.ThemeCharm).Theme(true))

	batch, ok := c.enter()().(tea.BatchMsg)
	require.True(t, ok)

	var summary confirmSummaryMsg
	var tick spinner.TickMsg
	for _, cmd := range batch {
		switch msg := cmd().(type) {
		case confirmSummaryMsg:
			summary = msg
		case spinner.TickMsg:
			tick = msg
		}
	}

	frame := c.view()
	require.Contains(t, frame, "Gathering installation summary…")
	require.NotNil(t, c.tick(tick))
	require.NotEqual(t, frame, c.view())

	c.update(summary)
	require.Nil(t, c.tick(tick))
	require.Contains(t, c.view(), "New Helm deployment will be created")
}

// A summary that failed is a change the wizard could not preview;
// Apply must stay unavailable.
func TestConfirmReady(t *testing.T) {
	tests := map[string]struct {
		loading bool
		err     error
		want    bool
	}{
		"loading":        {loading: true, want: false},
		"summary loaded": {want: true},
		"summary failed": {err: errors.New("kubernetes cluster unreachable"), want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := newConfirmModel(&stubBackend{}, huh.ThemeFunc(huh.ThemeCharm).Theme(true))
			c.loading, c.err = tt.loading, tt.err

			require.Equal(t, tt.want, c.ready())
			require.Equal(t, tt.want, strings.Contains(c.footer(), "apply"))
		})
	}
}
