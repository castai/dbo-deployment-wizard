package tui

import (
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/samber/lo"
)

// fetchSeq stamps background loads; it is global because screens are
// rebuilt on entry and a stale result may reach a newer instance.
var fetchSeq atomic.Int64

// asyncResultMsg delivers a background load's outcome; fetch names
// the load it answers.
type asyncResultMsg[T any] struct {
	value T
	err   error
	fetch int64
}

// async runs one background load at a time with a spinner standing in
// while it runs; results of superseded loads are dropped.
type async[T any] struct {
	spinner spinner.Model
	busy    bool
	fetch   int64
	value   T
	err     error
}

func newAsync[T any](styles *huh.Styles) async[T] {
	return async[T]{spinner: newSpinner(styles)}
}

// start resets the outcome and runs load in a background command.
func (a *async[T]) start(load func() (T, error)) tea.Cmd {
	var zero T
	a.busy, a.value, a.err = true, zero, nil
	a.fetch = fetchSeq.Add(1)
	fetch := a.fetch

	return tea.Batch(a.spinner.Tick, func() tea.Msg {
		value, err := load()

		return asyncResultMsg[T]{value: value, err: err, fetch: fetch}
	})
}

// apply takes msg's outcome if it answers the current load.
func (a *async[T]) apply(msg tea.Msg) bool {
	res, ok := msg.(asyncResultMsg[T])
	if !ok || res.fetch != a.fetch {
		return false
	}
	a.busy, a.value, a.err = false, res.value, res.err

	return true
}

// tick animates the spinner, re-arming it only while the load runs.
func (a *async[T]) tick(msg spinner.TickMsg) tea.Cmd {
	if !a.busy {
		return nil
	}
	var cmd tea.Cmd
	a.spinner, cmd = a.spinner.Update(msg)

	return cmd
}

func (a *async[T]) loading() bool {
	return a.busy
}

// renderError renders err line by line in the theme's error style.
func renderError(styles *huh.Styles, err error) string {
	lines := strings.Split(err.Error(), "\n")

	return strings.Join(lo.Map(lines, func(line string, _ int) string {
		return styles.Focused.ErrorMessage.Render(line)
	}), "\n")
}
