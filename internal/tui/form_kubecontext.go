package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// newKubeContextForm builds the kube-context form over the discovered
// contexts; the list is guaranteed non-empty (NewWizard fails fast
// otherwise).
func newKubeContextForm(theme huh.Theme, contexts api.Backend, d *draft) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Kubectl context").
				Description("Pick a Kubernetes cluster to install the Helm chart into.").
				OptionsFunc(func() []huh.Option[string] {
					contexts, _ := contexts.KubeContexts()

					return lo.Map(contexts, func(c string, _ int) huh.Option[string] {
						return huh.NewOption(c, c).Selected(c == d.kubeContext)
					})
				}, d.kubeContext).
				Value(&d.kubeContext),
		),
	).WithTheme(theme)
}

func (m *Model) handleKubeContext(msg tea.KeyPressMsg) tea.Cmd {
	cmd, done := forwardToForm(&m.kubeContextForm, msg)
	if done {
		m.completeForm()
	}

	return cmd
}
