package tui

import (
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
func newCredsForm(b api.Backend, initial api.Credentials, title string, set func(api.Credentials)) formSpec {
	creds := &initial
	const modeSecret = "secret"
	const modeUser = "user"

	pickedSecretMode := lo.Ternary(creds.SecretName != "" || creds.Username == "", modeSecret, modeUser)
	modePtr := lo.ToPtr(pickedSecretMode)

	fallbackToManualEdit := false

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(
					huh.NewOption("Existing Kubernetes Secret", modeSecret).Selected(pickedSecretMode == modeSecret),
					huh.NewOption("Username and password", modeUser).Selected(pickedSecretMode == modeSecret),
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
				if *modePtr != modeSecret || fallbackToManualEdit {
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
				return *modePtr != modeSecret || !fallbackToManualEdit
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
				if *modePtr != modeUser {
					return true
				}
				creds.SecretName = ""

				return false
			}),
	)

	return formSpec{
		form: form,
		commit: func() error {
			set(*creds)

			return nil
		},
	}
}

func newAgentCredsForm(b api.Backend) formSpec {
	return newCredsForm(b, b.AgentCredentials(), "Agent credentials", b.SetAgentCredentials)
}

func newPoolingCredsForm(b api.Backend) formSpec {
	return newCredsForm(b, b.PoolingCredentials(), "Pooling credentials", b.SetPoolingCredentials)
}
