package tui

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/samber/lo"

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
	target screenID
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
	value := truncate(ri.value, 60)
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
// summary, or — when the credentials are missing — the "required"
// marker (second return reports the warning).
func credsSub(c api.Credentials) (string, bool) {
	switch {
	case c.SecretName != "":
		return "Selected secret: " + c.SecretName, false
	case c.Username != "" && c.Password != "":
		return "Username: " + c.Username + ", password: " + maskPassword(c.Password), false
	default:
		return requiredSub, true
	}
}

// requiredSub marks a prerequisite the user still has to provide;
// rendered yellow by the review delegate.
const requiredSub = "⚠ required"

// maskedPasswordMax caps the bullet count so long passwords don't blow
// up the review row.
const maskedPasswordMax = 16

func maskPassword(pw string) string {
	return strings.Repeat("•", min(len(pw), maskedPasswordMax))
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

// The titles while discovery and the release lookup are in flight.
const (
	reviewTitleDiscovering = "Detecting current kubectl context and latest chart version…"
	reviewTitleLoading     = "Checking for an existing Helm deployment…"
)

// reviewTitle names whether the committed coordinates hold an
// existing release the install would upgrade, or none — a new
// installation it would create.
func reviewTitle(exists bool, releaseName string) string {
	if exists {
		return "Updating Helm deployment: " + releaseName
	}

	return "Installing new Helm deployment: " + releaseName
}

// discovery is the outcome of detecting the defaults for the
// coordinates the CLI left unset; latestVersion also labels the
// chart-version row.
type discovery struct {
	kubeContext      string
	kubeContextErr   error
	latestVersion    string
	latestVersionErr error
}

// coordinateSub is a coordinate row's sub line while its value is
// unset: nothing while discovery runs, else why discovery failed,
// else the "required" marker.
func coordinateSub(value string, discovering bool, err error) (string, bool) {
	switch {
	case value != "" || discovering:
		return "", false
	case err != nil:
		return "⚠ " + truncate(err.Error(), 100), true
	default:
		return requiredSub, true
	}
}

// rowState is what the rows show beyond the backend's committed state.
type rowState struct {
	discovery   discovery
	discovering bool
	// reviewErr is the backend's Continue validation message; it
	// rides the Continue action and renders above its button.
	reviewErr string
}

// reviewItems builds the review rows from the backend's committed
// state and s.
func reviewItems(b api.Backend, s rowState) []list.Item {
	agentCreds := b.AgentCredentials()
	pooling := b.PoolingCredentials()

	kubeContext := reviewItem{label: "Kubectl context:", value: emptyDash(b.KubeContext()), target: screenKubeContext}
	kubeContext.sub, kubeContext.subWarn = coordinateSub(b.KubeContext(), s.discovering, s.discovery.kubeContextErr)
	version := reviewItem{
		label:  "Chart version:",
		value:  chartVersionDisplay(b.ChartVersion(), s.discovery.latestVersion),
		target: screenChartVersion,
	}
	version.sub, version.subWarn = coordinateSub(b.ChartVersion(), s.discovering, s.discovery.latestVersionErr)

	rows := []reviewItem{
		kubeContext,
		{label: "Target namespace:", value: emptyDash(b.Namespace()), target: screenNamespace},
		version,
	}

	components := reviewItem{
		label:  "Features:",
		value:  emptyDash(componentNameList(b.Components())),
		target: screenComponents,
	}
	if len(b.Components()) == 0 {
		components.sub, components.subWarn = requiredSub, true
	}
	rows = append(rows, components)

	// Each credentials row exists only while its component is enabled.
	if slices.Contains(b.Components(), api.ComponentDBAgent) {
		row := reviewItem{
			label:  componentName(api.ComponentDBAgent) + " credentials:",
			value:  credsSummary(agentCreds),
			target: screenAgentCreds,
		}
		row.sub, row.subWarn = credsSub(agentCreds)
		rows = append(rows, row)
	}

	if slices.Contains(b.Components(), api.ComponentPooling) {
		row := reviewItem{
			label:  componentName(api.ComponentPooling) + " credentials:",
			value:  credsSummary(pooling),
			target: screenPoolingCreds,
		}
		row.sub, row.subWarn = credsSub(pooling)
		rows = append(rows, row)
	}

	rows = append(rows, reviewItem{
		label:  "Continue",
		action: true,
		err:    s.reviewErr,
	})

	return lo.Map(rows, func(r reviewItem, _ int) list.Item { return r })
}

// newItemList builds an item list with the wizard's shared chrome:
// no title, status bar, help, filtering, or quit bindings.
func newItemList(styles *huh.Styles, isDark bool) list.Model {
	l := list.New(nil, newReviewDelegate(styles, isDark), 80, 14)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.DisableQuitKeybindings()
	// The list's own filter would claim Esc, which quits the wizard here.
	l.SetFilteringEnabled(false)
	l.InfiniteScrolling = true

	return l
}

// reviewModel is the review screen: the configuration hub list with
// the Continue action. It owns the list, the validation message riding
// Continue, and the screen's rendering.
type reviewModel struct {
	backend api.Backend
	styles  *huh.Styles
	list    list.Model

	// reviewError is the backend's Continue validation (or a failed
	// commit's) message, rendered above the Continue action; reset
	// when the user returns from a section (the edit may have fixed it).
	reviewError string

	// defaults detects the unset coordinates, once per run; release is
	// the deferred DeploymentExists lookup.
	defaults   async[discovery]
	discovered bool
	release    async[bool]
}

func newReviewModel(b api.Backend, styles *huh.Styles, isDark bool) *reviewModel {
	r := &reviewModel{
		backend:  b,
		styles:   styles,
		list:     newItemList(styles, isDark),
		defaults: newAsync[discovery](styles),
		release:  newAsync[bool](styles),
	}
	r.sync()

	// Start on the Continue action
	r.list.Select(len(r.list.Items()) - 1)

	return r
}

// sync rebuilds the review rows and sizes the list so every row lands
// on one page: the list pages at availHeight/(rowHeight+spacing), so
// the height is 2 rows per item (with slack for the delegates' sub
// lines) and never below 14.
func (r *reviewModel) sync() {
	selected, hadSelection := r.list.SelectedItem().(reviewItem)

	items := reviewItems(r.backend, rowState{
		discovery:   r.defaults.value,
		discovering: r.defaults.loading(),
		reviewErr:   r.reviewError,
	})
	r.list.SetItems(items)

	h := 2*len(items) + 2
	if h < 14 {
		h = 14
	}
	r.list.SetHeight(h)

	// The list keeps the cursor's index, but the credentials rows come
	// and go — follow the row instead.
	_, i, found := lo.FindIndexOf(items, func(it list.Item) bool {
		ri, ok := it.(reviewItem)

		return ok && ri.target == selected.target && ri.action == selected.action
	})
	if hadSelection && found {
		r.list.Select(i)
	}
}

// reset prepares the rows for re-entry from a section, showing the
// section's failed commit, if any.
func (r *reviewModel) reset(commitErr error) {
	r.reviewError = ""
	if commitErr != nil {
		r.reviewError = commitErr.Error()
	}
}

// Init detects the defaults once per run, then re-checks the release —
// the edit may have changed its coordinates. Both reach out, so they
// run in the background to keep the UI (Ctrl+C included) responsive.
func (r *reviewModel) Init() tea.Cmd {
	if r.discovered {
		r.sync()

		return r.lookupRelease()
	}
	r.discovered = true
	// Read on the UI goroutine, which owns the committed state.
	b, needContext := r.backend, r.backend.KubeContext() == ""
	cmd := r.defaults.start(func() (discovery, error) {
		var d discovery
		if needContext {
			d.kubeContext, d.kubeContextErr = b.CurrentKubeContext()
		}
		d.latestVersion, d.latestVersionErr = b.LatestChartVersion()

		return d, nil
	})
	r.sync()

	return cmd
}

// applyDefaults commits the detected defaults for whatever is still
// unset; a section the user edited meanwhile wins.
func (r *reviewModel) applyDefaults() {
	d := r.defaults.value
	if d.kubeContext != "" && r.backend.KubeContext() == "" {
		r.backend.SetKubeContext(d.kubeContext)
	}
	if d.latestVersion != "" && r.backend.ChartVersion() == "" {
		r.backend.SetChartVersion(d.latestVersion)
	}
}

// lookupRelease re-checks the release; without a context there is
// nothing to check.
func (r *reviewModel) lookupRelease() tea.Cmd {
	if r.backend.KubeContext() == "" {
		return nil
	}

	return r.release.start(r.backend.DeploymentExists)
}

//nolint:ireturn // screen.Update returns the next screen.
func (r *reviewModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case asyncResultMsg[discovery]:
		if r.defaults.apply(msg) {
			r.applyDefaults()
			r.sync()

			return r, r.lookupRelease()
		}
	case asyncResultMsg[bool]:
		r.release.apply(msg)
	case spinner.TickMsg:
		return r, tea.Batch(r.defaults.tick(msg), r.release.tick(msg))
	case tea.KeyPressMsg:
		return r, r.handleKey(msg)
	}

	return r, nil
}

// handleKey: Esc quits, Enter enters the selected section or — from
// Continue, once Validate passes — moves on to confirmation.
func (r *reviewModel) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	if isEsc(msg) {
		return send(abortMsg{})
	}
	if !isConfirmAction(msg) {
		var cmd tea.Cmd
		r.list, cmd = r.list.Update(msg)

		return cmd
	}
	it, ok := r.list.SelectedItem().(reviewItem)
	switch {
	case !ok:
		return nil
	case !it.action:
		return navigate(it.target)
	}
	if err := r.backend.Validate(); err != nil {
		r.reviewError = err.Error()
		r.sync()

		return nil
	}

	return navigate(screenConfirm)
}

// view renders the whole screen: title, rows, help footer.
func (r *reviewModel) View() string {
	var title string
	switch {
	case r.defaults.loading():
		title = r.styles.Group.Title.Render(r.defaults.spinner.View() + " " + reviewTitleDiscovering)
	case r.backend.KubeContext() == "":
		title = r.styles.Group.Title.Render("Helm deployment: " + r.backend.ReleaseName())
	case r.release.loading():
		title = r.styles.Group.Title.Render(r.release.spinner.View() + " " + reviewTitleLoading)
	case r.release.err != nil:
		title = renderError(r.styles, r.release.err)
	default:
		title = r.styles.Group.Title.Render(reviewTitle(r.release.value, r.backend.ReleaseName()))
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		r.list.View(),
		r.styles.Help.ShortDesc.Render(r.footer()),
	)
}

// footer builds the review help line; the Enter hint names the action
// the cursor is on.
func (r *reviewModel) footer() string {
	enterHint := "edit"
	if it, ok := r.list.SelectedItem().(reviewItem); ok && it.action {
		enterHint = "proceed to confirmation"
	}

	parts := []string{
		keyChip(r.styles, "↑/↓") + " pick option to edit",
		keyChip(r.styles, "Enter") + " " + enterHint,
		keyChip(r.styles, "Esc") + " quit",
	}

	return strings.Join(parts, "  ")
}
