package tui

import (
	"charm.land/bubbles/v2/key"
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

// navigateMsg asks the root to replace the active screen; err is a
// failed commit to show on review.
type navigateMsg struct {
	to  screenID
	err error
}

func navigate(to screenID) tea.Cmd {
	return send(navigateMsg{to: to})
}

// abortMsg asks the root to quit without installing.
type abortMsg struct{}

// formDoneMsg is a form's SubmitCmd: the adapter commits the draft.
type formDoneMsg struct{}

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

// formScreen adapts a huh form to a screen: huh reports completion and
// cancel through SubmitCmd/CancelCmd; completion commits the draft.
type formScreen struct {
	formSpec
	theme  huh.Theme
	keymap *huh.KeyMap
}

func newFormScreen(theme huh.Theme, spec formSpec) *formScreen {
	s := &formScreen{formSpec: spec, theme: theme}
	s.adopt(spec.form)

	return s
}

func (s *formScreen) adopt(f *huh.Form) {
	s.keymap = huh.NewDefaultKeyMap()
	s.keymap.Quit = key.NewBinding(key.WithKeys(append(s.keymap.Quit.Keys(), "esc")...))
	s.form = f.WithTheme(s.theme).WithKeyMap(s.keymap)
	s.form.SubmitCmd = send(formDoneMsg{})
	s.form.CancelCmd = navigate(screenReview)
}

func (s *formScreen) Init() tea.Cmd {
	return s.form.Init()
}

//nolint:ireturn // screen.Update returns the next screen.
func (s *formScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if _, ok := msg.(formDoneMsg); ok {
		return s, send(navigateMsg{to: screenReview, err: s.commit()})
	}

	keyMsg, isKey := msg.(tea.KeyPressMsg)
	if isKey {
		// huh matches Quit before the focused field sees the key, so
		// Esc would cancel even while a select uses it for its filter.
		s.keymap.Quit.SetEnabled(!fieldClaimsKey(s.form.GetFocusedField(), keyMsg))
	}

	model, cmd := s.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		s.form = f
	}
	if isKey && s.rebuild != nil && s.form.State == huh.StateNormal {
		if f := s.rebuild(); f != nil {
			s.adopt(f)

			return s, s.form.Init()
		}
	}

	return s, cmd
}

// fieldClaimsKey reports whether the field has an enabled binding of
// its own for msg, e.g. a select's Esc to set or clear its filter.
func fieldClaimsKey(f huh.Field, msg tea.KeyPressMsg) bool {
	return f != nil && key.Matches(msg, f.KeyBinds()...)
}

// View renders the form directly; huh draws its own title and help
// bar, so no page chrome is added.
func (s *formScreen) View() string {
	return s.form.View()
}
