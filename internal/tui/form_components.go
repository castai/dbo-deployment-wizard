package tui

import (
	"slices"

	"charm.land/huh/v2"
	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// newComponentsForm builds the components screen. huh offers no
// disabled options or toggle veto, so the pooling⇒db-proxy lock is
// enforced after every key: a toggle that breaks it drops pooling from
// the draft and rebuilds the form.
func newComponentsForm(b api.Backend) formSpec {
	selected := b.Components()

	return formSpec{
		form: componentsForm(&selected),
		commit: func() error {
			return b.SetComponents(selected)
		},
		rebuild: func() *huh.Form {
			reconciled := reconcileComponents(selected)
			if slices.Equal(reconciled, selected) {
				return nil
			}
			selected = reconciled

			return componentsForm(&selected)
		},
	}
}

// componentsForm builds the components MultiSelect. Height is set
// explicitly because huh's auto-sizing would collapse the field to a
// single visible row.
func componentsForm(selected *[]string) *huh.Form {
	options := lo.Map(api.AllComponents, func(c string, _ int) huh.Option[string] {
		return huh.NewOption(c, c).Selected(slices.Contains(*selected, c))
	})

	return huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Components").
				Description("Chart subcharts to install; pooling requires db-proxy.").
				Options(options...).
				Value(selected).
				Height(len(api.AllComponents) + 3),
		).Title("Components"),
	)
}

// reconcileComponents drops pooling whenever db-proxy is not selected,
// so an invalid combination never survives a toggle.
func reconcileComponents(sel []string) []string {
	if slices.Contains(sel, api.ComponentPooling) && !slices.Contains(sel, api.ComponentDBProxy) {
		return slices.DeleteFunc(slices.Clone(sel), func(x string) bool { return x == api.ComponentPooling })
	}

	return sel
}
