package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// page composes the layout every screen uses: title → body → help
// footer.
func (m *Model) page(title, body, footer string) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.styles.Group.Title.Render(title),
		body,
		m.styles.Help.ShortDesc.Render(footer),
	)
}

// helpFooter returns the help line for the given screen; on review the
// Enter hint names the action the cursor is on.
func (m *Model) helpFooter(s screen) string {
	parts := []string{m.keyChip("↑/↓") + " move"}
	if s == screenReview {
		enterHint := "edit"
		if it, ok := m.reviewList.SelectedItem().(reviewItem); ok && it.action {
			enterHint = "proceed to installation"
		}
		parts = append(parts,
			m.keyChip("Enter")+" "+enterHint,
			m.keyChip("Esc")+" quit",
		)
	}

	return strings.Join(parts, "  ")
}

func (m *Model) keyChip(k string) string {
	return m.styles.Help.ShortKey.Render(" " + k + " ")
}

// View dispatches on m.state and wraps the rendered body in the
// alt-screen tea.View.
//
//nolint:ireturn // bubbletea v2 Model.View mandates a tea.View return.
func (m *Model) View() tea.View {
	var body string
	switch m.state {
	case screenReview:
		body = m.viewReview()
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

func (m *Model) viewReview() string {
	return m.page("Installing castai-dbo helm chart — review configuration", m.reviewList.View(), m.helpFooter(m.state))
}

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
