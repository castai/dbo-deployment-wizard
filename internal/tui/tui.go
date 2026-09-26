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

	"charm.land/bubbles/v2/list"
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

// Model is the bubbletea model. Sub-models (the review list, the
// embedded huh forms) own their own cursor, scroll, and focus state;
// per-screen handlers only intercept keys the sub-model can't handle
// (Enter to commit, Esc to go back, Ctrl+C to abort).
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

	reviewList list.Model

	// Embedded huh forms, one per screen; huh forms are single-use, so
	// primeScreen rebuilds them on every entry.
	kubeContextForm  *huh.Form
	chartVersionForm *huh.Form
	namespaceForm    *huh.Form
	componentsForm   *huh.Form
	agentCredsForm   *huh.Form
	poolingCredsForm *huh.Form

	// agentCredsMode / poolingCredsMode are the sources the credentials
	// forms' selects picked; the group hide funcs read them live and
	// completeForm uses them to pick the committed source.
	agentCredsMode   string
	poolingCredsMode string

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
	m.styles = m.theme.Theme(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))

	m.reviewList = list.New(nil, reviewDelegate{theme: m.styles}, 80, 14)
	m.reviewList.SetShowHelp(false)
	m.reviewList.SetShowStatusBar(false)
	m.reviewList.SetShowTitle(false)
	m.reviewList.DisableQuitKeybindings()
	m.refreshReviewList()

	m.syncDraft()
	m.buildForms(nil)

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

// buildForms builds every embedded form; the credentials screens
// re-discover Secrets on entry in primeScreen.
func (m *Model) buildForms(secrets []string) {
	m.kubeContextForm = newKubeContextForm(m.theme, m.backend.KubeContexts(), &m.draft)
	m.chartVersionForm = newChartVersionForm(m.theme, m.backend.ChartVersions(), &m.draft)
	m.namespaceForm = newNamespaceForm(m.theme, &m.draft)
	m.componentsForm = newComponentsForm(m.theme, &m.draft)
	m.agentCredsForm = newCredsForm(m.theme, &m.agentCredsMode, secrets, &m.draft.agentCreds, "Agent credentials")
	m.poolingCredsForm = newCredsForm(m.theme, &m.poolingCredsMode, secrets, &m.draft.poolingCreds, "Pooling credentials")
}

// primeScreen re-syncs the draft and rebuilds the screen's form (huh
// forms are single-use).
func (m *Model) primeScreen(s screen) {
	switch s {
	case screenReview:
		m.refreshReviewList()
	case screenKubeContext:
		m.syncDraft()
		m.kubeContextForm = newKubeContextForm(m.theme, m.backend.KubeContexts(), &m.draft)
	case screenChartVersion:
		m.syncDraft()
		m.chartVersionForm = newChartVersionForm(m.theme, m.backend.ChartVersions(), &m.draft)
	case screenAgentCreds:
		m.syncDraft()
		m.agentCredsForm = newCredsForm(m.theme, &m.agentCredsMode, m.backend.Secrets(), &m.draft.agentCreds, "Agent credentials")
	case screenComponents:
		m.syncDraft()
		m.componentsForm = newComponentsForm(m.theme, &m.draft)
	case screenNamespace:
		m.syncDraft()
		m.namespaceForm = newNamespaceForm(m.theme, &m.draft)
	case screenPoolingCreds:
		m.syncDraft()
		m.poolingCredsForm = newCredsForm(m.theme, &m.poolingCredsMode, m.backend.Secrets(), &m.draft.poolingCreds, "Pooling credentials")
	}
}

// Init is part of tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Update intercepts KeyPressMsg and background-color messages; every
// other message — window sizes, and bracketed paste, which arrives as
// PasteMsg rather than KeyPressMsg — reaches the active form so it can
// size itself and apply pastes.
//
//nolint:ireturn // bubbletea v2 Model.Update mandates a tea.Model return.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kpm, ok := msg.(tea.KeyPressMsg); ok {
		mm, cmd := m.handleKey(kpm)

		return mm, cmd
	}
	if bgm, ok := msg.(tea.BackgroundColorMsg); ok {
		_, _ = forwardToForm(&m.kubeContextForm, bgm)
		_, _ = forwardToForm(&m.poolingCredsForm, bgm)
		_, _ = forwardToForm(&m.chartVersionForm, bgm)
		_, _ = forwardToForm(&m.namespaceForm, bgm)
		_, _ = forwardToForm(&m.componentsForm, bgm)
		_, _ = forwardToForm(&m.agentCredsForm, bgm)

		return m, nil
	}
	if f := m.activeForm(); f != nil {
		cmd, done := forwardToForm(f, msg)
		if done {
			m.completeForm()
		}

		return m, cmd
	}

	return m, nil
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
	}

	return nil
}

// refreshReviewList rebuilds the review rows and sizes the list so
// every row lands on one page: the list pages at
// availHeight/(rowHeight+spacing), so the height is 2 rows per item
// (with slack for the delegates' sub lines) and never below 14.
func (m *Model) refreshReviewList() {
	items := reviewItems(m.backend)
	m.reviewList.SetItems(items)

	h := 2*len(items) + 2
	if h < 14 {
		h = 14
	}
	m.reviewList.SetHeight(h)
}

func (m *Model) goToReview() {
	m.refreshReviewList()
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
		m.backend.SetAgentCredentials(credentialsFromDraft(m.draft.agentCreds, m.agentCredsMode))
	case screenPoolingCreds:
		m.backend.SetPoolingCredentials(credentialsFromDraft(m.draft.poolingCreds, m.poolingCredsMode))
	}
	m.goToReview()
}

// credentialsFromDraft keeps only the source the form's select picked,
// so the committed struct never carries both.
func credentialsFromDraft(c api.Credentials, mode string) api.Credentials {
	if mode == credsModeUserPassLabel {
		return api.Credentials{Username: c.Username, Password: c.Password}
	}

	return api.Credentials{SecretName: c.SecretName}
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
		return b.Install()
	}

	return ErrAborted
}
