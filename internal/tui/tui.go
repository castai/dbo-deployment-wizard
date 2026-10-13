// Package tui implements the deployment wizard's interactive UI: a
// bubbletea review-list hub with embedded huh forms for every section.
// It is pure UI over the api.Backend contract: state for display comes
// from queries, actions go through commands. Forms bind to a per-entry
// draft, never to the backend's committed configuration.
package tui

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// ErrAborted is returned when the user aborts with Ctrl+C or Esc; the
// cmd entry detects it via errors.Is and prints "install aborted".
var ErrAborted = errors.New("install aborted")

// Model is the bubbletea model: the screen router. It holds exactly
// one active screen, routes every message to it, and carries out the
// navigation the screens ask for through messages.
type Model struct {
	// backend is the service-layer contract: the single owner of the
	// committed configuration.
	backend api.Backend

	// theme is shared with the embedded forms via WithTheme; styles is
	// its resolution for the terminal background.
	theme  huh.Theme
	styles *huh.Styles

	active screen

	// review outlives navigation so the hub keeps its cursor when the
	// user returns from a section.
	review *reviewModel

	done   bool
	cancel bool
}

// isConfirmAction reports whether msg is a plain Enter key.
func isConfirmAction(msg tea.KeyPressMsg) bool {
	k := msg.Key()

	return k.Code == tea.KeyEnter && k.Mod == 0
}

// isEsc reports whether msg is a plain Esc key.
func isEsc(msg tea.KeyPressMsg) bool {
	k := msg.Key()

	return k.Code == tea.KeyEsc && k.Mod == 0
}

func newModel(b api.Backend) *Model {
	return newModelWithBackground(b, lipgloss.HasDarkBackground(os.Stdin, os.Stdout))
}

func newModelWithBackground(b api.Backend, isDark bool) *Model {
	m := &Model{backend: b}

	m.theme = huh.ThemeFunc(huh.ThemeCharm)
	m.styles = m.theme.Theme(isDark)

	m.review = newReviewModel(b, m.styles, isDark)
	m.active = m.review

	return m
}

// newScreen is the single screen factory: forms are rebuilt on every
// entry because a huh form is single-use.
//
//nolint:ireturn // the factory returns whichever screen nav targets.
func (m *Model) newScreen(nav navigateMsg) screen {
	switch nav.to {
	case screenKubeContext:
		return newFormScreen(m.theme, newKubeContextForm(m.backend))
	case screenNamespace:
		return newFormScreen(m.theme, newNamespaceForm(m.backend))
	case screenChartVersion:
		return newFormScreen(m.theme, newChartVersionForm(m.backend))
	case screenAgentCreds:
		return newFormScreen(m.theme, newAgentCredsForm(m.backend))
	case screenComponents:
		return newFormScreen(m.theme, newComponentsForm(m.backend))
	case screenPoolingCreds:
		return newFormScreen(m.theme, newPoolingCredsForm(m.backend))
	case screenConfirm:
		return newConfirmModel(m.backend, m.styles)
	default:
		m.review.reset(nav.err)

		return m.review
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(repaintIn(), m.active.Init())
}

//nolint:ireturn // bubbletea v2 Model.Update mandates a tea.Model return.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case repaintMsg:
		return m, tea.Batch(tea.ClearScreen, repaintIn())
	case tea.KeyPressMsg:
		if typed.String() == "ctrl+c" {
			m.cancel = true

			return m, tea.Quit
		}
	case navigateMsg:
		m.active = m.newScreen(typed)

		return m, tea.Batch(tea.ClearScreen, m.active.Init())
	case abortMsg:
		m.cancel = true

		return m, tea.Quit
	case applyMsg:
		m.done = true

		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.active, cmd = m.active.Update(msg)

	return m, cmd
}

// Run starts the bubbletea program. The install itself runs after the
// program exits, on the real terminal, so helm's output streams
// normally.
func Run(b api.Backend) error {
	m := newModel(b)
	finalModel, err := tea.NewProgram(m).Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	fm, ok := finalModel.(*Model)
	if !ok {
		return fmt.Errorf("tui: unexpected final model type %T", finalModel)
	}
	if fm.done {
		return b.Install(os.Stdout)
	}

	return ErrAborted
}
