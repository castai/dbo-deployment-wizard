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

// screen identifies which screen the TUI is currently rendering.
type screen int

const (
	screenReview screen = iota
	screenKubeContext
	screenNamespace
	screenChartVersion
	screenAgentCreds
	screenComponents
	screenPoolingCreds
	screenConfirm
)

// draft is the per-entry working state the forms bind to: initialized
// from the backend on screen entry, committed on completion, discarded
// on Esc.
type draft struct {
	kubeContext  string
	namespace    string
	chartVersion string
	components   []string
	agentCreds   api.Credentials
	poolingCreds api.Credentials
}

// Model is the bubbletea model: the screen router. The review and
// confirm screens and the embedded huh forms are sub-models owning
// their own state and rendering; the root routes keys to the active
// one and carries out the navigation they ask for.
type Model struct {
	state screen

	// backend is the service-layer contract: the single owner of the
	// committed configuration.
	backend api.Backend

	// theme is shared with the embedded forms via WithTheme; styles is
	// its resolution for the terminal background.
	theme  huh.Theme
	styles *huh.Styles

	// draft is the working state of the screen being edited.
	draft draft

	review  *reviewModel
	confirm *confirmModel

	kubeContextForm  *huh.Form
	chartVersionForm *huh.Form
	namespaceForm    *huh.Form
	componentsForm   *huh.Form
	agentCredsForm   *huh.Form
	poolingCredsForm *huh.Form

	done   bool
	cancel bool
}

// isConfirmAction reports whether msg is a plain Enter key.
func isConfirmAction(msg tea.KeyPressMsg) bool {
	k := msg.Key()

	return k.Code == tea.KeyEnter && k.Mod == 0
}

func newModel(b api.Backend) *Model {
	m := &Model{
		state:   screenReview,
		backend: b,
	}

	m.theme = huh.ThemeFunc(huh.ThemeCharm)
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	m.styles = m.theme.Theme(isDark)

	m.review = newReviewModel(b, m.styles, isDark)
	m.confirm = newConfirmModel(b, m.styles)

	m.syncDraft()
	m.buildForms()

	return m
}

func (m *Model) syncDraft() {
	m.draft = draft{
		kubeContext:  m.backend.KubeContext(),
		namespace:    m.backend.Namespace(),
		chartVersion: m.backend.ChartVersion(),
		components:   m.backend.Components(),
		agentCreds:   m.backend.AgentCredentials(),
		poolingCreds: m.backend.PoolingCredentials(),
	}
}

// buildForms builds every embedded form; the credentials screens'
// Secret discovery is huh's dynamic options (see newCredsForm).
func (m *Model) buildForms() {
	m.kubeContextForm = newKubeContextForm(m.theme, m.backend, &m.draft)
	m.chartVersionForm = newChartVersionForm(m.theme, m.backend, &m.draft)
	m.namespaceForm = newNamespaceForm(m.theme, &m.draft)
	m.componentsForm = newComponentsForm(m.theme, &m.draft)
	m.agentCredsForm = newCredsForm(m.theme, m.backend, &m.draft.agentCreds, "Agent credentials")
	m.poolingCredsForm = newCredsForm(m.theme, m.backend, &m.draft.poolingCreds, "Pooling credentials")
}

func (m *Model) setCurrentScreen(s screen) {
	m.state = s
	switch s {
	case screenKubeContext:
		m.syncDraft()
		m.kubeContextForm = newKubeContextForm(m.theme, m.backend, &m.draft)
	case screenChartVersion:
		m.syncDraft()
		m.chartVersionForm = newChartVersionForm(m.theme, m.backend, &m.draft)
	case screenAgentCreds:
		m.syncDraft()
		m.agentCredsForm = newCredsForm(m.theme, m.backend, &m.draft.agentCreds, "Agent credentials")
	case screenComponents:
		m.syncDraft()
		m.componentsForm = newComponentsForm(m.theme, &m.draft)
	case screenNamespace:
		m.syncDraft()
		m.namespaceForm = newNamespaceForm(m.theme, &m.draft)
	case screenPoolingCreds:
		m.syncDraft()
		m.poolingCredsForm = newCredsForm(m.theme, m.backend, &m.draft.poolingCreds, "Pooling credentials")
	}
}

func (m *Model) Init() tea.Cmd {
	return repaintIn()
}

//nolint:ireturn // bubbletea v2 Model.Update mandates a tea.Model return.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.state
	var cmd tea.Cmd

	switch typed := msg.(type) {
	case repaintMsg:
		cmd = tea.Batch(tea.ClearScreen, repaintIn())
	case tea.KeyPressMsg:
		_, cmd = m.handleKey(typed)
	case confirmSummaryMsg:
		m.confirm.update(typed)
	case tea.BackgroundColorMsg:
		_, _ = forwardToForm(&m.kubeContextForm, typed)
		_, _ = forwardToForm(&m.poolingCredsForm, typed)
		_, _ = forwardToForm(&m.chartVersionForm, typed)
		_, _ = forwardToForm(&m.namespaceForm, typed)
		_, _ = forwardToForm(&m.componentsForm, typed)
		_, _ = forwardToForm(&m.agentCredsForm, typed)
	default:
		if f := m.activeForm(); f != nil {
			var done bool
			cmd, done = forwardToForm(f, msg)
			if done {
				m.completeForm()
			}
		}
	}

	if m.state != before {
		cmd = tea.Batch(cmd, tea.ClearScreen)
	}

	return m, cmd
}

// handleKey is the central dispatcher: Ctrl+C aborts from anywhere;
// Esc quits from review, backs out of sub-screens.
//
//nolint:ireturn // bubbletea v2 Model.Update mandates a tea.Model return.
func (m *Model) handleKey(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	k := msg.Key()

	if k.Mod&tea.ModCtrl != 0 {
		m.cancel = true

		return m, tea.Quit
	}

	if k.Code == tea.KeyEsc && k.Mod == 0 {
		if m.state == screenReview {
			m.cancel = true

			return m, tea.Quit
		}
		m.goToReview()

		return m, nil
	}

	return m, m.currentViewHandleKey(msg)
}

func (m *Model) currentViewHandleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch m.state {
	case screenReview:
		return m.handleReview(msg)
	case screenKubeContext:
		return m.handleKubeContext(msg)
	case screenNamespace:
		return m.handleNamespace(msg)
	case screenChartVersion:
		return m.handleChartVersion(msg)
	case screenAgentCreds:
		return m.handleAgentCreds(msg)
	case screenComponents:
		return m.handleComponents(msg)
	case screenPoolingCreds:
		return m.handlePoolingCreds(msg)
	case screenConfirm:
		return m.handleConfirm(msg)
	}

	return nil
}

// handleReview routes the review sub-model's keys and carries out the
// navigation it asks for: entering the selected section, or — from
// Continue — moving on to confirmation.
func (m *Model) handleReview(msg tea.KeyPressMsg) tea.Cmd {
	cmd, next := m.review.update(msg)
	if next == screenReview {
		return cmd
	}

	m.setCurrentScreen(next)
	if next == screenConfirm {
		return m.confirm.enter()
	}
	if f := m.activeForm(); f != nil {
		// Init focuses the form's first field.
		return tea.Batch(cmd, (*f).Init())
	}

	return cmd
}

// handleConfirm: Enter applies once the confirm sub-model's summary
// has loaded — the program quits and the install continues in plain
// CLI mode. Enter while loading does nothing, so a double-click
// through from review can never apply a summary the user has not
// seen.
func (m *Model) handleConfirm(msg tea.KeyPressMsg) tea.Cmd {
	if isConfirmAction(msg) && !m.confirm.loading {
		m.done = true

		return tea.Quit
	}

	return nil
}

func (m *Model) goToReview() {
	m.review.backToReview()
	m.state = screenReview
}

// forwardToForm routes a message into an embedded huh form and reports
// whether the form finished.
func forwardToForm(f **huh.Form, msg tea.Msg) (tea.Cmd, bool) {
	if *f == nil {
		return nil, false
	}
	model, cmd := (*f).Update(msg)
	form, ok := model.(*huh.Form)
	if !ok {
		return cmd, false
	}
	*f = form

	return cmd, form.State != huh.StateNormal
}

// activeForm returns the form embedded in the current screen, if any.
func (m *Model) activeForm() **huh.Form {
	switch m.state {
	case screenKubeContext:
		return &m.kubeContextForm
	case screenChartVersion:
		return &m.chartVersionForm
	case screenNamespace:
		return &m.namespaceForm
	case screenComponents:
		return &m.componentsForm
	case screenAgentCreds:
		return &m.agentCredsForm
	case screenPoolingCreds:
		return &m.poolingCredsForm
	}

	return nil
}

// completeForm commits the completed form's draft through the
// backend's commands — the UI's single write path.
func (m *Model) completeForm() {
	switch m.state {
	case screenKubeContext:
		m.backend.SetKubeContext(m.draft.kubeContext)
	case screenChartVersion:
		m.backend.SetChartVersion(m.draft.chartVersion)
	case screenNamespace:
		m.backend.SetNamespace(m.draft.namespace)
	case screenComponents:
		_ = m.backend.SetComponents(m.draft.components)
	case screenAgentCreds:
		m.backend.SetAgentCredentials(m.draft.agentCreds)
	case screenPoolingCreds:
		m.backend.SetPoolingCredentials(m.draft.poolingCreds)
	}
	m.goToReview()
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
