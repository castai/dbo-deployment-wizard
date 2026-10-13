package tui

import (
	"strings"

	"charm.land/huh/v2"
	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// versionLabel renders "1.4.2 (latest, released 2026-01-15)" for the
// newest version, "1.4.1 (released 2025-12-20)" for the rest.
func versionLabel(v api.HelmChartVersion, latest bool) string {
	parts := make([]string, 0, 2)
	if latest {
		parts = append(parts, "latest")
	}
	if !v.Created.IsZero() {
		parts = append(parts, "released "+v.Created.Format("2006-01-02"))
	}
	if len(parts) == 0 {
		return v.Number
	}

	return v.Number + " (" + strings.Join(parts, ", ") + ")"
}

// newChartVersionForm builds the chart-version form with release-date
// labels.
func newChartVersionForm(b api.Backend) formSpec {
	version := b.ChartVersion()

	return formSpec{
		form: huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Chart version").
					Description("Pick a Helm chart version to use.").
					OptionsFunc(func() []huh.Option[string] {
						versions, _ := b.ChartVersions()

						return lo.Map(versions, func(v api.HelmChartVersion, i int) huh.Option[string] {
							return huh.NewOption(versionLabel(v, i == 0), v.Number).Selected(v.Number == version)
						})
					}, version).
					Value(&version),
			),
		),
		commit: func() error {
			b.SetChartVersion(version)

			return nil
		},
	}
}
