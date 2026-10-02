package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// keyChip renders a key hint chip for the screens' help footers.
func keyChip(styles *huh.Styles, k string) string {
	return styles.Help.ShortKey.Render(" " + k + " ")
}

// View dispatches on m.state and wraps the rendered body in the
// alt-screen tea.View.
//
//nolint:ireturn // bubbletea v2 Model.View mandates a tea.View return.
func (m *Model) View() tea.View {
	var body string
	switch m.state {
	case screenReview:
		body = m.review.view()
	case screenKubeContext:
		body = m.viewKubeContext()
	case screenNamespace:
		body = m.viewNamespace()
	case screenChartVersion:
		body = m.viewChartVersion()
	case screenAgentCreds:
		body = m.viewAgentCreds()
	case screenComponents:
		body = m.viewComponents()
	case screenPoolingCreds:
		body = m.viewPoolingCreds()
	case screenConfirm:
		body = m.confirm.view()
	default:
		body = "(unknown screen)"
	}

	return wrapView(body)
}

func wrapView(body string) tea.View {
	v := tea.NewView(body)
	// Alt screen seems to cause issues in AWS and GCP cloud shells
	v.AltScreen = false
	v.MouseMode = tea.MouseModeNone

	return v
}

// The form screens render the embedded huh form directly; huh draws
// its own title and help bar, so no page chrome is added.

func (m *Model) viewKubeContext() string {
	return m.kubeContextForm.View()
}

func (m *Model) viewNamespace() string {
	return m.namespaceForm.View()
}

func (m *Model) viewChartVersion() string {
	return m.chartVersionForm.View()
}

func (m *Model) viewAgentCreds() string {
	return m.agentCredsForm.View()
}

func (m *Model) viewComponents() string {
	return m.componentsForm.View()
}

func (m *Model) viewPoolingCreds() string {
	return m.poolingCredsForm.View()
}
