package tui

import (
	"fmt"
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
	if s == screenConfirm {
		return strings.Join([]string{
			m.keyChip("Enter") + " apply",
			m.keyChip("Esc") + " back",
		}, "  ")
	}

	parts := []string{m.keyChip("↑/↓") + " pick option to edit"}
	if s == screenReview {
		enterHint := "edit"
		if it, ok := m.reviewList.SelectedItem().(reviewItem); ok && it.action {
			enterHint = "proceed to confirmation"
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
	case screenConfirm:
		body = m.viewConfirm()
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
	return m.page(reviewTitle(m.backend), m.reviewList.View(), m.helpFooter(m.state))
}

func (m *Model) viewConfirm() string {
	var body strings.Builder
	for _, line := range m.confirmLines {
		fmt.Fprintln(&body, m.styles.Focused.File.Render(line))
	}
	if m.confirmErr != nil {
		for _, line := range strings.Split(m.confirmErr.Error(), "\n") {
			fmt.Fprintln(&body, m.styles.Focused.ErrorMessage.Render(line))
		}
	}

	fmt.Fprintln(&body)
	fmt.Fprint(&body, m.styles.Focused.FocusedButton.Render("Apply"))

	return m.page("Ready to install", body.String(), m.helpFooter(m.state))
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
