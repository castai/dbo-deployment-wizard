package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// Some terminal emulators (browser cloud shells, JetBrains' built-in
// terminal) mishandle the cell renderer's incremental writes during
// rapid updates, leaving their cursor model desynced and the screen
// garbled. A periodic full repaint re-anchors the renderer; terminals
// that handle incremental updates correctly just see a few redundant
// writes.
const repaintInterval = 60 * time.Millisecond

// repaintMsg re-arms the full-repaint loop.
type repaintMsg struct{}

// repaintIn schedules the next full repaint.
func repaintIn() tea.Cmd {
	return tea.Tick(repaintInterval, func(time.Time) tea.Msg {
		return repaintMsg{}
	})
}
