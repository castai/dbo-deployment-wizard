package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// newComponentsForm builds the components MultiSelect. Height is set
// explicitly because huh's auto-sizing would collapse the field to a
// single visible row.
func newComponentsForm(theme huh.Theme, d *draft) *huh.Form {
	options := lo.Map(api.AllComponents, func(c string, _ int) huh.Option[string] {
		return huh.NewOption(c, c).Selected(slices.Contains(d.components, c))
	})

	return huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Components").
				Description("Chart subcharts to install; pooling requires db-proxy.").
				Options(options...).
				Value(&d.components).
				Height(len(api.AllComponents) + 3),
		).Title("Components"),
	).WithTheme(theme)
}

// reconcileComponents drops pooling whenever db-proxy is not selected,
// so an invalid combination never survives a toggle.
func reconcileComponents(sel []string) []string {
	if slices.Contains(sel, api.ComponentPooling) && !slices.Contains(sel, api.ComponentDBProxy) {
		return slices.DeleteFunc(slices.Clone(sel), func(x string) bool { return x == api.ComponentPooling })
	}

	return sel
}

// handleComponents enforces the pooling⇒db-proxy lock on the draft
// after every key (huh offers no disabled options or toggle veto):
// a toggle that breaks the lock drops pooling from the draft and
// rebuilds the form.
func (m *Model) handleComponents(msg tea.KeyPressMsg) tea.Cmd {
	cmd, done := forwardToForm(&m.componentsForm, msg)
	if done {
		m.completeForm()

		return cmd
	}
	if reconciled := reconcileComponents(m.draft.components); !slices.Equal(reconciled, m.draft.components) {
		m.draft.components = reconciled
		m.componentsForm = newComponentsForm(m.theme, &m.draft)

		return m.componentsForm.Init()
	}

	return cmd
}
