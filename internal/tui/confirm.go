package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// confirmModel is the confirmation screen: the
// installation-impact summary and the Apply action. A background
// command loads the summary; Enter is gated until it lands, so a
// double-click from review can never apply unseen.
type confirmModel struct {
	backend api.Backend
	styles  *huh.Styles
	summary async[[]string]
}

func newConfirmModel(b api.Backend, styles *huh.Styles) *confirmModel {
	return &confirmModel{backend: b, styles: styles, summary: newAsync[[]string](styles)}
}

// Init defers the summary to a background command so entering is
// instant.
func (c *confirmModel) Init() tea.Cmd {
	return c.summary.start(c.backend.SummarizeInstallationImpact)
}

//nolint:ireturn // screen.Update returns the next screen.
func (c *confirmModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case asyncResultMsg[[]string]:
		c.summary.apply(msg)
	case spinner.TickMsg:
		return c, c.summary.tick(msg)
	case tea.KeyPressMsg:
		return c, c.handleKey(msg)
	}

	return c, nil
}

// handleKey: Enter applies once the summary has loaded — the program
// quits and the install continues in plain CLI mode. Enter while
// loading does nothing, so a double-click through from review can
// never apply a summary the user has not seen; nor does it when the
// summary failed.
func (c *confirmModel) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case isEsc(msg):
		return navigate(screenReview)
	case isConfirmAction(msg) && c.ready():
		return send(applyMsg{})
	}

	return nil
}

// ready reports whether Apply is available: only once the summary has
// loaded and succeeded, so the user never applies a change the wizard
// could not preview.
func (c *confirmModel) ready() bool {
	return !c.summary.loading() && c.summary.err == nil
}

// view renders the whole screen: title, summary, Apply button, help
// footer.
func (c *confirmModel) View() string {
	// The blank lines mirror the review screen's list padding.
	var body strings.Builder
	fmt.Fprintln(&body)
	if c.summary.loading() {
		fmt.Fprintln(&body, c.summary.spinner.View()+" "+c.styles.Focused.Description.Render("Gathering installation summary…"))
	} else {
		for _, line := range c.summary.value {
			fmt.Fprintln(&body, c.styles.Focused.File.Render(line))
		}
		if c.summary.err != nil {
			fmt.Fprintln(&body, renderError(c.styles, c.summary.err))
		}
	}

	fmt.Fprintln(&body)
	button := c.styles.Focused.FocusedButton
	if !c.ready() {
		button = c.styles.Blurred.BlurredButton
	}
	fmt.Fprintln(&body, button.Render("Apply"))
	fmt.Fprintln(&body)

	return lipgloss.JoinVertical(lipgloss.Left,
		c.styles.Group.Title.Render("Ready to install"),
		body.String(),
		c.styles.Help.ShortDesc.Render(c.footer()),
	)
}

// footer builds the help line; the Enter hint appears once Apply is
// available.
func (c *confirmModel) footer() string {
	parts := []string{}
	if c.ready() {
		parts = append(parts, keyChip(c.styles, "Enter")+" apply")
	}
	parts = append(parts, keyChip(c.styles, "Esc")+" back")

	return strings.Join(parts, "  ")
}
