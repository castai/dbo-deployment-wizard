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
	r := require.New(t)

	c := newConfirmModel(&stubBackend{}, huh.ThemeFunc(huh.ThemeCharm).Theme(true))

	batch, ok := c.enter()().(tea.BatchMsg)
	r.True(ok)

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
	r.Contains(frame, "Gathering installation summary…")
	r.NotNil(c.tick(tick))
	r.NotEqual(frame, c.view())

	c.update(summary)
	r.Nil(c.tick(tick))
	r.Contains(c.view(), "New Helm deployment will be created")
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
			r := require.New(t)

			c := newConfirmModel(&stubBackend{}, huh.ThemeFunc(huh.ThemeCharm).Theme(true))
			c.loading, c.err = tt.loading, tt.err

			r.Equal(tt.want, c.ready())
			r.Equal(tt.want, strings.Contains(c.footer(), "apply"))
		})
	}
}
