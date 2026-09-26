package tui

import (
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// Credential source labels.
//
//nolint:gosec // G101: UI labels, not credentials.
const (
	credsModeSecretLabel   = "Existing Kubernetes Secret"
	credsModeUserPassLabel = "Username and password"
)

// newCredsForm builds the multi-step credentials form: the source
// select, then the picked source's fields revealed via the group hide
// funcs. The secret picker loads its options through huh's dynamic
// options: discovery runs asynchronously with a spinner while the
// existing-Secret group is active, so a slow cluster never blocks the
// form and the username/password path never triggers it; an empty
// discovery hides the picker and shows a free-text name input instead.
// The fields carry no required-validation — huh blocks group
// navigation while a group has errors, which would trap the user in
// the username/password step; completeness is enforced by the review
// screen's Continue gate instead.
func newCredsForm(theme huh.Theme, mode *string, b api.Backend, creds *api.Credentials, title string) *huh.Form {
	def := credsModeSecretLabel
	if creds.SecretName == "" && creds.Username != "" {
		def = credsModeUserPassLabel
	}
	*mode = def

	// noSecrets flips when discovery finds nothing, swapping the
	// picker group for a free-text input; the hide funcs read it live.
	// Atomic: the options function runs off the UI thread.
	var noSecrets atomic.Bool

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Source").
				Description("How the component sources its database credentials.").
				Options(
					huh.NewOption(credsModeSecretLabel, credsModeSecretLabel).Selected(def == credsModeSecretLabel),
					huh.NewOption(credsModeUserPassLabel, credsModeUserPassLabel).Selected(def == credsModeUserPassLabel),
				).
				Value(mode),
		).Title(title),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select secret").
				Description("Pick from existing secrets in namespace "+b.Namespace()).
				OptionsFunc(func() []huh.Option[string] {
					secrets := b.Secrets()
					noSecrets.Store(len(secrets) == 0)

					return lo.Map(secrets, func(s string, _ int) huh.Option[string] {
						return huh.NewOption(s, s).Selected(s == creds.SecretName)
					})
				}, mode).
				Value(&creds.SecretName),
		).
			Title(title).
			WithHideFunc(func() bool { return *mode != credsModeSecretLabel || noSecrets.Load() }),

		huh.NewGroup(
			huh.NewInput().
				Title("Secret name").
				Description("No Secrets discovered in namespace "+b.Namespace()+"; enter the name.").
				Value(&creds.SecretName),
		).
			Title(title).
			WithHideFunc(func() bool { return *mode != credsModeSecretLabel || !noSecrets.Load() }),

		huh.NewGroup(
			huh.NewInput().
				Title("Username").
				Value(&creds.Username),
			huh.NewInput().
				Title("Password").
				EchoMode(huh.EchoModePassword).
				Value(&creds.Password),
		).
			Title(title).
			WithHideFunc(func() bool { return *mode != credsModeUserPassLabel }),
	).WithTheme(theme)
}

func (m *Model) handleAgentCreds(msg tea.KeyPressMsg) tea.Cmd {
	cmd, done := forwardToForm(&m.agentCredsForm, msg)
	if done {
		m.completeForm()
	}

	return cmd
}

func (m *Model) handlePoolingCreds(msg tea.KeyPressMsg) tea.Cmd {
	f := m.activeForm()
	if f == nil {
		return nil
	}
	cmd, done := forwardToForm(f, msg)
	if done {
		m.completeForm()
	}

	return cmd
}
