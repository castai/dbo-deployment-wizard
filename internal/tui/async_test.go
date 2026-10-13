package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/stretchr/testify/require"
)

// loadResult runs the load started by cmd and returns its result
// message, skipping the spinner tick.
func loadResult(tb testing.TB, cmd tea.Cmd) asyncResultMsg[string] {
	tb.Helper()
	r := require.New(tb)

	sub, ok := cmdList(cmd())
	r.True(ok)
	for _, msg := range collect(sub) {
		if res, ok := msg.(asyncResultMsg[string]); ok {
			return res
		}
	}
	r.FailNow("no result")

	return asyncResultMsg[string]{}
}

func TestAsync(t *testing.T) {
	loadErr := errors.New("boom")
	tests := map[string]struct {
		value     string
		err       error
		stale     bool
		wantApply bool
	}{
		"current result applies":  {value: "v", wantApply: true},
		"error is carried":        {err: loadErr, wantApply: true},
		"superseded load dropped": {value: "old", stale: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			a := newAsync[string](huh.ThemeCharm(true))

			res := loadResult(t, a.start(func() (string, error) { return tt.value, tt.err }))
			if tt.stale {
				_ = a.start(func() (string, error) { return "new", nil })
			}
			r.True(a.loading())

			r.Equal(tt.wantApply, a.apply(res))
			r.Equal(!tt.wantApply, a.loading())
			if tt.wantApply {
				r.Equal(tt.value, a.value)
				r.Equal(tt.err, a.err)
			} else {
				r.Empty(a.value)
			}
		})
	}
}

func TestAsync_IgnoresOtherMessages(t *testing.T) {
	r := require.New(t)
	a := newAsync[string](huh.ThemeCharm(true))
	_ = a.start(func() (string, error) { return "", nil })

	r.False(a.apply(asyncResultMsg[bool]{fetch: a.fetch}))
	r.True(a.loading())
}
