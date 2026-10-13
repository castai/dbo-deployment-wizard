package tui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// keyChip renders a key hint chip for the screens' help footers.
func keyChip(styles *huh.Styles, k string) string {
	return styles.Help.ShortKey.Render(" " + k + " ")
}

// newSpinner matches the one huh shows while a select's options load.
func newSpinner(styles *huh.Styles) spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.Line),
		spinner.WithStyle(styles.Focused.MultiSelectSelector.UnsetString()),
	)
}

// View wraps the active screen in the inline tea.View.
//
//nolint:ireturn // bubbletea v2 Model.View mandates a tea.View return.
func (m *Model) View() tea.View {
	return wrapView(m.active.View())
}

func wrapView(body string) tea.View {
	v := tea.NewView(body)
	// Alt screen seems to cause issues in AWS and GCP cloud shells
	v.AltScreen = false
	v.MouseMode = tea.MouseModeNone

	return v
}
