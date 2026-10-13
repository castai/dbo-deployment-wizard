package tui

import (
	"sync"

	"charm.land/huh/v2"
	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// newKubeContextForm builds the kube-context form over the discovered
// contexts; the description reports a failed lookup.
func newKubeContextForm(b api.Backend) formSpec {
	kubeContext := b.KubeContext()
	// Captured here: the options func runs off the UI goroutine, which
	// owns the bound value.
	current := kubeContext
	discover := sync.OnceValues(b.KubeContexts)

	return formSpec{
		form: huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Kubectl context").
					DescriptionFunc(func() string {
						_, err := discover()

						return discoveryDescription(err, "kubectl contexts", "Pick a Kubernetes cluster to install the Helm chart into.")
					}, current).
					OptionsFunc(func() []huh.Option[string] {
						contexts, _ := discover()

						return lo.Map(contexts, func(c string, _ int) huh.Option[string] {
							return huh.NewOption(c, c).Selected(c == current)
						})
					}, current).
					Value(&kubeContext),
			),
		),
		commit: func() error {
			b.SetKubeContext(kubeContext)

			return nil
		},
	}
}
