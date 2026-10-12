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

// confirmModel is the confirmation screen's sub-model: the
// installation-impact summary and the Apply action. A background
// command loads the summary; Enter is gated until it lands, so a
// double-click from review can never apply unseen.
type confirmModel struct {
	backend api.Backend
	styles  *huh.Styles
	lines   []string
	err     error
	loading bool
	spinner spinner.Model

	// fetch stamps each entry so stale responses drop.
	fetch int
}

func newConfirmModel(b api.Backend, styles *huh.Styles) *confirmModel {
	return &confirmModel{backend: b, styles: styles, spinner: newSpinner(styles)}
}

// confirmSummaryMsg delivers the screen's deferred summary; fetch
// identifies which entry's request it answers.
type confirmSummaryMsg struct {
	lines []string
	err   error
	fetch int
}

// enter resets the screen and defers the summary to a background
// command so entering is instant; the fetch stamp lets update drop
// responses superseded by a newer entry.
func (c *confirmModel) enter() tea.Cmd {
	c.loading = true
	c.lines, c.err = nil, nil
	c.fetch++
	b, fetch := c.backend, c.fetch

	return tea.Batch(c.spinner.Tick, func() tea.Msg {
		lines, err := b.SummarizeInstallationImpact()

		return confirmSummaryMsg{lines: lines, err: err, fetch: fetch}
	})
}

// tick animates the spinner, re-arming it only while the summary loads.
func (c *confirmModel) tick(msg spinner.TickMsg) tea.Cmd {
	if !c.loading {
		return nil
	}
	var cmd tea.Cmd
	c.spinner, cmd = c.spinner.Update(msg)

	return cmd
}

// update applies a deferred summary, dropping responses a newer entry
// has superseded.
func (c *confirmModel) update(msg confirmSummaryMsg) {
	if msg.fetch != c.fetch {
		return
	}
	c.loading = false
	c.lines, c.err = msg.lines, msg.err
}

// ready reports whether Apply is available: only once the summary has
// loaded and succeeded, so the user never applies a change the wizard
// could not preview.
func (c *confirmModel) ready() bool {
	return !c.loading && c.err == nil
}

// view renders the whole screen: title, summary, Apply button, help
// footer.
func (c *confirmModel) view() string {
	// The blank lines mirror the review screen's list padding.
	var body strings.Builder
	fmt.Fprintln(&body)
	if c.loading {
		fmt.Fprintln(&body, c.spinner.View()+" "+c.styles.Focused.Description.Render("Gathering installation summary…"))
	} else {
		for _, line := range c.lines {
			fmt.Fprintln(&body, c.styles.Focused.File.Render(line))
		}
		if c.err != nil {
			for _, line := range strings.Split(c.err.Error(), "\n") {
				fmt.Fprintln(&body, c.styles.Focused.ErrorMessage.Render(line))
			}
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
