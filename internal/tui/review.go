package tui

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// reviewItem is the review list's item: a config section (label,
// value, optional sub line) or — with action set — the Continue
// install trigger.
type reviewItem struct {
	label    string
	value    string
	sub      string
	subError bool
	target   screen
	action   bool
}

func (i reviewItem) FilterValue() string { return i.label + " " + i.value }

// reviewDelegate renders reviewItem rows through the huh theme.
type reviewDelegate struct{ theme *huh.Styles }

func (reviewDelegate) Height() int { return 1 }

func (reviewDelegate) Spacing() int { return 1 }

func (reviewDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d reviewDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ri, ok := item.(reviewItem)
	if !ok {
		return
	}
	cursor := d.theme.Focused.SelectSelector.String()
	if ri.action {
		// The Continue item renders as the theme's action button.
		style := d.theme.Blurred.BlurredButton
		if index == m.Index() {
			fmt.Fprint(w, cursor)
			style = d.theme.Focused.FocusedButton
		} else {
			fmt.Fprint(w, strings.Repeat(" ", lipgloss.Width(cursor)))
		}
		fmt.Fprint(w, style.Render(ri.label))

		return
	}
	value := truncate(ri.value, 48)
	if index == m.Index() {
		row := d.theme.Focused.Title.Render(ri.label) + " " +
			d.theme.Focused.SelectedOption.Render(value)
		fmt.Fprint(w, cursor+row)
	} else {
		row := d.theme.Form.Base.Render(ri.label) + " " +
			d.theme.Focused.File.Render(value)
		fmt.Fprint(w, strings.Repeat(" ", lipgloss.Width(cursor))+row)
	}
	if ri.sub != "" {
		fmt.Fprintln(w)
		sub := "       " + ri.sub
		if ri.subError {
			fmt.Fprint(w, d.theme.Focused.ErrorMessage.Render(sub))
		} else {
			fmt.Fprint(w, d.theme.Focused.Description.Render(sub))
		}
	}
}

// unsetPlaceholder renders unset review values.
const unsetPlaceholder = "—"

// credsSummary returns how credentials are sourced: the existing
// Secret, the username/password pair, or the unset placeholder.
func credsSummary(c api.Credentials) string {
	switch {
	case c.SecretName != "":
		return "existing secret"
	case c.Username != "" && c.Password != "":
		return "username/password"
	default:
		return unsetPlaceholder
	}
}

// credsSub renders the credentials row's sub line.
func credsSub(c api.Credentials) (string, bool) {
	switch {
	case c.SecretName != "":
		return "Selected secret: " + c.SecretName, true
	case c.Username != "" && c.Password != "":
		return "Username: " + c.Username + ", password: " + maskPassword(c.Password), true
	default:
		return "", false
	}
}

// poolingCredsComplete reports whether the pooling credentials are
// provided; trivially true when pooling is not selected. The review
// screen gates its Continue action on it.
func poolingCredsComplete(b api.Backend) bool {
	return !slices.Contains(b.Components(), api.ComponentPooling) ||
		b.PoolingCredentials().Provided()
}

// maskedPasswordMax caps the bullet count so long passwords don't blow
// up the review row.
const maskedPasswordMax = 16

func maskPassword(pw string) string {
	return strings.Repeat("•", max(len(pw), maskedPasswordMax))
}

func emptyDash(s string) string {
	if s == "" {
		return unsetPlaceholder
	}

	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n-1] + "…"
}

// chartVersionDisplay suffixes " (latest)" when the configured version
// is the newest available.
func chartVersionDisplay(version, latestVersion string) string {
	if version != "" && version == latestVersion {
		return version + " (latest)"
	}

	return emptyDash(version)
}

// reviewItems builds the review rows from the backend's committed
// state.
func reviewItems(b api.Backend) []list.Item {
	agentCreds := b.AgentCredentials()
	pooling := b.PoolingCredentials()
	latestVersion := b.LatestChartVersion()
	rows := []reviewItem{
		{label: "Kubectl context:", value: emptyDash(b.KubeContext()), target: screenKubeContext},
		{label: "Target namespace:", value: emptyDash(b.Namespace()), target: screenNamespace},
		{label: "Chart version:", value: chartVersionDisplay(b.ChartVersion(), latestVersion), target: screenChartVersion},
		{label: "Agent credentials:", value: credsSummary(agentCreds), target: screenAgentCreds},
		{label: "Components to install:", value: emptyDash(strings.Join(b.Components(), ", ")), target: screenComponents},
	}

	if sub, ok := credsSub(agentCreds); ok {
		for i := range rows {
			if rows[i].target == screenAgentCreds {
				rows[i].sub = sub

				break
			}
		}
	}

	if slices.Contains(b.Components(), api.ComponentPooling) {
		row := reviewItem{
			label:  "Pooling credentials:",
			value:  credsSummary(pooling),
			target: screenPoolingCreds,
		}
		if sub, ok := credsSub(pooling); ok {
			row.sub = sub
		} else {
			row.sub = "⚠ not provided"
			row.subError = true
		}
		rows = append(rows, row)
	}

	rows = append(rows, reviewItem{
		label:  "Continue",
		action: true,
	})

	out := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, r)
	}

	return out
}

// handleReview: Enter enters the selected section, or — on the
// Continue item — finalizes the install, gated on
// poolingCredsComplete.
func (m *Model) handleReview(msg tea.KeyPressMsg) tea.Cmd {
	if isConfirmAction(msg) {
		if it, ok := m.reviewList.SelectedItem().(reviewItem); ok {
			if it.action {
				if !poolingCredsComplete(m.backend) {
					return nil
				}
				m.done = true

				return nil
			}
			m.state = it.target
			m.primeScreen(it.target)
			if f := m.activeForm(); f != nil {
				// Init focuses the form's first field.
				return (*f).Init()
			}
		}

		return nil
	}

	var cmd tea.Cmd
	m.reviewList, cmd = m.reviewList.Update(msg)

	return cmd
}
