package tui

import (
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// screen is one page of the wizard; the root holds exactly one and
// routes every message to it.
type screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (screen, tea.Cmd)
	View() string
}

// screenID names a navigation target for the screen factory.
type screenID int

const (
	screenReview screenID = iota
	screenKubeContext
	screenNamespace
	screenChartVersion
	screenAgentCreds
	screenComponents
	screenPoolingCreds
	screenConfirm
)

// navigateMsg asks the root to replace the active screen.
type navigateMsg struct {
	to screenID
}

func navigate(to screenID) tea.Cmd {
	return send(navigateMsg{to: to})
}

// abortMsg asks the root to quit without installing.
type abortMsg struct{}

// applyMsg asks the root to quit and run the install.
type applyMsg struct{}

func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}

// formSpec is what a leaf form constructor returns: the form bound to
// its per-entry draft and the commit that writes the draft through the
// backend.
type formSpec struct {
	form   *huh.Form
	commit func() error
	// rebuild runs after every key press; a non-nil form replaces the
	// current one (huh offers no toggle veto, so draft fix-ups need a
	// fresh form).
	rebuild func() *huh.Form
}

// formScreen adapts a huh form to a screen: it commits on completion
// and returns to review either way.
type formScreen struct {
	formSpec
	theme huh.Theme
}

func newFormScreen(theme huh.Theme, spec formSpec) *formScreen {
	s := &formScreen{formSpec: spec, theme: theme}
	s.adopt(spec.form)

	return s
}

func (s *formScreen) adopt(f *huh.Form) {
	s.form = f.WithTheme(s.theme)
}

func (s *formScreen) Init() tea.Cmd {
	return s.form.Init()
}

//nolint:ireturn // screen.Update returns the next screen.
func (s *formScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	key, isKey := msg.(tea.KeyPressMsg)
	if isKey && isEsc(key) {
		return s, navigate(screenReview)
	}

	model, cmd := s.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		s.form = f
	}
	if s.form.State == huh.StateCompleted {
		_ = s.commit()

		return s, navigate(screenReview)
	}
	if isKey && s.rebuild != nil {
		if f := s.rebuild(); f != nil {
			s.adopt(f)

			return s, s.form.Init()
		}
	}

	return s, cmd
}

// View renders the form directly; huh draws its own title and help
// bar, so no page chrome is added.
func (s *formScreen) View() string {
	return s.form.View()
}

// fetchSeq stamps background loads; it is global because screens are
// rebuilt on entry and a stale result may reach a newer instance.
var fetchSeq atomic.Int64

func nextFetch() int64 { return fetchSeq.Add(1) }
