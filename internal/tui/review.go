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
	label string
	value string
	sub   string
	// subWarn renders sub as the yellow "required" marker.
	subWarn bool
	// err is the Continue item's validation message, rendered
	// above the button.
	err    string
	target screen
	action bool
}

func (i reviewItem) FilterValue() string { return i.label + " " + i.value }

// reviewDelegate renders reviewItem rows through the huh theme.
type reviewDelegate struct {
	theme *huh.Styles
	// warn styles the "required" markers and err the Continue
	// validation message; huh ships neither as a theme style, so
	// both are built light/dark-aware in newReviewDelegate.
	warn, err lipgloss.Style
}

// newReviewDelegate builds the review delegate's marker styles. The
// yellows differ per background for readability; err reuses the
// huh Charm theme's error reds so the message matches form errors.
func newReviewDelegate(styles *huh.Styles, isDark bool) reviewDelegate {
	lightDark := lipgloss.LightDark(isDark)

	return reviewDelegate{
		theme: styles,
		warn:  lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("130"), lipgloss.Color("214"))),
		err:   lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("#FF4672"), lipgloss.Color("#ED567A"))),
	}
}

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
		if ri.err != "" {
			indent := strings.Repeat(" ", lipgloss.Width(cursor))
			for _, line := range strings.Split(ri.err, "\n") {
				fmt.Fprintln(w, indent+d.err.Render(line))
			}
		}
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
		if ri.subWarn {
			fmt.Fprint(w, d.warn.Render(sub))
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

// credsSub renders the credentials row's sub line: the source
// summary, or — when the credentials are a prerequisite and missing —
// the "required" marker (second return reports the warning).
func credsSub(c api.Credentials, required bool) (string, bool) {
	switch {
	case c.SecretName != "":
		return "Selected secret: " + c.SecretName, false
	case c.Username != "" && c.Password != "":
		return "Username: " + c.Username + ", password: " + maskPassword(c.Password), false
	case required:
		return requiredSub, true
	default:
		return "", false
	}
}

// requiredSub marks a prerequisite the user still has to provide;
// rendered yellow by the review delegate.
const requiredSub = "⚠ required"

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

// reviewTitle renders the review screen's title; the suffix names
// whether the committed coordinates hold an existing release the
// install would upgrade, or none — a new installation it would
// create — or, when the lookup fails, why.
func reviewTitle(b api.Backend) string {
	exists, err := b.DeploymentExists()
	switch {
	case err != nil:
		message := truncate(firstLine(err.Error()), 48)
		return fmt.Sprintf("Failed to determine if installing or upgrading %s: %s ", b.ReleaseName(), message)
	case exists:
		return "Installing Helm deployment " + b.ReleaseName()
	default:
		return "Updating Helm deployment " + b.ReleaseName()
	}
}

// firstLine clips multi-line strings (kubectl errors) to their first line.
func firstLine(s string) string {
	before, _, _ := strings.Cut(s, "\n")
	return before
}

// reviewItems builds the review rows from the backend's committed
// state; reviewErr — the backend's Continue validation message —
// rides the Continue action and renders above its button.
func reviewItems(b api.Backend, reviewErr string) []list.Item {
	agentCreds := b.AgentCredentials()
	pooling := b.PoolingCredentials()
	latestVersion := b.LatestChartVersion()

	// The agent credentials are only a prerequisite when db-agent is
	// enabled; pooling's row exists only when pooling is.
	agentRow := reviewItem{
		label:  "Agent credentials:",
		value:  credsSummary(agentCreds),
		target: screenAgentCreds,
	}
	agentRow.sub, agentRow.subWarn = credsSub(agentCreds, slices.Contains(b.Components(), api.ComponentDBAgent))

	rows := []reviewItem{
		{label: "Kubectl context:", value: emptyDash(b.KubeContext()), target: screenKubeContext},
		{label: "Target namespace:", value: emptyDash(b.Namespace()), target: screenNamespace},
		{label: "Chart version:", value: chartVersionDisplay(b.ChartVersion(), latestVersion), target: screenChartVersion},
		agentRow,
		{label: "Components to install:", value: emptyDash(strings.Join(b.Components(), ", ")), target: screenComponents},
	}

	if slices.Contains(b.Components(), api.ComponentPooling) {
		row := reviewItem{
			label:  "Pooling credentials:",
			value:  credsSummary(pooling),
			target: screenPoolingCreds,
		}
		row.sub, row.subWarn = credsSub(pooling, true)
		rows = append(rows, row)
	}

	rows = append(rows, reviewItem{
		label:  "Continue",
		action: true,
		err:    reviewErr,
	})

	out := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, r)
	}

	return out
}

// handleReview: Enter enters the selected section, or — on the
// Continue item — finalizes the install after the backend validates
// every prerequisite; a failure renders the message above the
// button instead of proceeding.
func (m *Model) handleReview(msg tea.KeyPressMsg) tea.Cmd {
	if isConfirmAction(msg) {
		if it, ok := m.reviewList.SelectedItem().(reviewItem); ok {
			if it.action {
				if err := m.backend.Validate(); err != nil {
					m.reviewError = err.Error()
					m.refreshReviewList()

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
