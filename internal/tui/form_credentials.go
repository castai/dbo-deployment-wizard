package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
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
func newCredsForm(theme huh.Theme, b api.Backend, creds *api.Credentials, title string) *huh.Form {
	const mode_secret = "secret"
	const mode_user = "user"

	pickedSecretMode := lo.Ternary(creds == nil || creds.SecretName != "" || creds.Username == "", mode_secret, mode_user)
	modePtr := lo.ToPtr(pickedSecretMode)

	fallbackToManualEdit := false

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(
					huh.NewOption("Existing Kubernetes Secret", mode_secret).Selected(pickedSecretMode == mode_secret),
					huh.NewOption("Username and password", mode_user).Selected(pickedSecretMode == mode_secret),
				).
				Value(modePtr),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Description("Pick from existing secrets in namespace "+b.Namespace()).
				OptionsFunc(func() []huh.Option[string] {
					secrets := b.Secrets()
					fallbackToManualEdit = len(secrets) == 0

					if fallbackToManualEdit {
						return []huh.Option[string]{
							huh.NewOption("No secrets found, continue to enter key manually...", "").Selected(true),
						}
					}

					return lo.Map(secrets, func(s string, _ int) huh.Option[string] {
						return huh.NewOption(s, s).Selected(s == creds.SecretName)
					})
				}, pickedSecretMode).
				Height(10).
				Value(&creds.SecretName),
		).
			WithHideFunc(func() bool {
				if !(*modePtr == mode_secret && !fallbackToManualEdit) {
					return true
				}
				creds.Password = ""
				creds.Username = ""
				return false
			}),

		huh.NewGroup(
			huh.NewInput().
				Title("Enter existing secret name. Must exist in namespace "+b.Namespace()).
				Value(&creds.SecretName),
		).
			Title(title).
			WithHideFunc(func() bool {
				return !(*modePtr == mode_secret && fallbackToManualEdit)
			}),

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
			WithHideFunc(func() bool {
				if *modePtr != mode_user {
					return true
				}
				creds.SecretName = ""
				return false
			}),
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
