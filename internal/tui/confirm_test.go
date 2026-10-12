package tui

import (
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
