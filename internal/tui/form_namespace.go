package tui

import (
	"charm.land/huh/v2"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

func newNamespaceForm(b api.Backend) formSpec {
	namespace := b.Namespace()

	return formSpec{
		form: huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("K8s Namespace").
					Description("Components will be installed to this Kubernetes namespace. Will be created if missing.").
					Value(&namespace),
			),
		),
		commit: func() error {
			b.SetNamespace(namespace)

			return nil
		},
	}
}
