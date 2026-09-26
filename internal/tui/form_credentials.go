package tui

import (
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
// funcs. The fields carry no required-validation — huh blocks group
// navigation while a group has errors, which would trap the user in
// the username/password step; completeness is enforced by the review
// screen's Continue gate instead.
func newCredsForm(theme huh.Theme, mode *string, secrets []string, creds *api.Credentials, title string) *huh.Form {
	def := credsModeSecretLabel
	if creds.SecretName == "" && creds.Username != "" {
		def = credsModeUserPassLabel
	}
	*mode = def

	var secretField huh.Field
	if len(secrets) > 0 {
		options := lo.Map(secrets, func(s string, _ int) huh.Option[string] {
			return huh.NewOption(s, s).Selected(s == creds.SecretName)
		})
		secretField = huh.NewSelect[string]().
			Title("Secret name").
			Description("Discovered in the target namespace.").
			Options(options...).
			Value(&creds.SecretName)
	} else {
		secretField = huh.NewInput().
			Title("Secret name").
			Description("No Secrets discovered in the target namespace; enter the name.").
			Value(&creds.SecretName)
	}

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

		huh.NewGroup(secretField).
			Title(title).
			WithHideFunc(func() bool { return *mode != credsModeSecretLabel }),

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
