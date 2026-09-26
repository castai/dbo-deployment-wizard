package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func newNamespaceForm(theme huh.Theme, d *draft) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("K8s Namespace").
				Description("Components will be installed to this Kubernetes namespace. Will be created if missing.").
				Value(&d.namespace),
		),
	).WithTheme(theme)
}

func (m *Model) handleNamespace(msg tea.KeyPressMsg) tea.Cmd {
	cmd, done := forwardToForm(&m.namespaceForm, msg)
	if done {
		m.completeForm()
	}

	return cmd
}
