package tui

import (
	"charm.land/huh/v2"
	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// newKubeContextForm builds the kube-context form over the discovered
// contexts; the list is guaranteed non-empty (NewWizard fails fast
// otherwise).
func newKubeContextForm(b api.Backend) formSpec {
	kubeContext := b.KubeContext()

	return formSpec{
		form: huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Kubectl context").
					Description("Pick a Kubernetes cluster to install the Helm chart into.").
					OptionsFunc(func() []huh.Option[string] {
						contexts, _ := b.KubeContexts()

						return lo.Map(contexts, func(c string, _ int) huh.Option[string] {
							return huh.NewOption(c, c).Selected(c == kubeContext)
						})
					}, kubeContext).
					Value(&kubeContext),
			),
		),
		commit: func() error {
			b.SetKubeContext(kubeContext)

			return nil
		},
	}
}
