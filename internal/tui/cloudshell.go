package tui

import (
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// browserCloudShell reports whether the terminal is a browser cloud shell
// (AWS/GCP/Azure): their xterm.js-inside-tmux stack mishandles the cell
// renderer's incremental writes — targeted scroll-region updates land on
// wrong rows and leave stale fragments — so only full repaints render
// cleanly. Detection is by env var: browser terminals report the same
// TERM as a native xterm, so TERM can't be used.
func browserCloudShell() bool {
	return os.Getenv("AWS_EXECUTION_ENV") == "CloudShell" ||
		os.Getenv("CLOUD_SHELL") == "true" ||
		strings.HasPrefix(os.Getenv("AZUREPS_HOST_ENVIRONMENT"), "cloud-shell/")
}

// cloudShellRepaintInterval is how often the full repaint fires: fast
// enough to mask incremental-write corruption, slow enough not to thrash.
const cloudShellRepaintInterval = 120 * time.Millisecond

// repaintMsg re-arms the full-repaint loop.
type repaintMsg struct{}

// cloudShellRepaint schedules the next full repaint.
func cloudShellRepaint() tea.Cmd {
	return tea.Tick(cloudShellRepaintInterval, func(time.Time) tea.Msg {
		return repaintMsg{}
	})
}
