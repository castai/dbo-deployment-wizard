package tui

import (
	"sync"

	"charm.land/huh/v2"
	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

const (
	credsModeSecret = "secret"
	credsModeUser   = "user"
)

// manualSecretLabel is the secret picker's option whose empty value
// reveals the free-text secret name input.
const manualSecretLabel = "Enter a name manually…"

// newCredsForm builds the multi-step credentials form: the source
// select, then the picked source's fields revealed via the group hide
// funcs. The secret picker loads its options through huh's dynamic
// options: discovery runs asynchronously with a spinner while the
// existing-Secret group is active, so a slow cluster never blocks the
// form and the username/password path never triggers it. Picking the
// manual option reveals a free-text name input instead.
// The fields carry no required-validation — huh blocks group
// navigation while a group has errors, which would trap the user in
// the username/password step; completeness is enforced by the review
// screen's Continue gate instead.
func newCredsForm(b api.Backend, initial api.Credentials, title string, set func(api.Credentials)) formSpec {
	creds := &initial
	namespace := b.Namespace()
	// Captured here: the options func runs off the UI goroutine, which
	// owns the bound creds.
	currentSecret := creds.SecretName

	pickedMode := lo.Ternary(creds.SecretName != "" || creds.Username == "", credsModeSecret, credsModeUser)
	mode := pickedMode

	// Options and description evaluate on huh's command goroutines; the
	// Once shares one discovery between them.
	discover := sync.OnceValues(b.Secrets)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(
					huh.NewOption("Existing Kubernetes Secret", credsModeSecret).Selected(pickedMode == credsModeSecret),
					huh.NewOption("Username and password", credsModeUser).Selected(pickedMode == credsModeUser),
				).
				Value(&mode),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				DescriptionFunc(func() string {
					secrets, err := discover()

					return secretsDescription(secrets, err, namespace)
				}, pickedMode).
				OptionsFunc(func() []huh.Option[string] {
					secrets, err := discover()

					return secretOptions(secrets, err, currentSecret)
				}, pickedMode).
				Height(10).
				Value(&creds.SecretName),
		).
			WithHideFunc(func() bool {
				return mode != credsModeSecret
			}),

		huh.NewGroup(
			huh.NewInput().
				Title("Enter existing secret name. Must exist in namespace "+namespace).
				Value(&creds.SecretName),
		).
			Title(title).
			WithHideFunc(func() bool {
				return mode != credsModeSecret || creds.SecretName != ""
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
				return mode != credsModeUser
			}),
	)

	return formSpec{
		form: form,
		commit: func() error {
			set(credsForMode(*creds, mode))

			return nil
		},
	}
}

// credsForMode keeps only the picked source's fields.
func credsForMode(creds api.Credentials, mode string) api.Credentials {
	if mode == credsModeUser {
		creds.SecretName = ""
	} else {
		creds.Username, creds.Password = "", ""
	}

	return creds
}

// secretOptions lists the discovered secrets plus the manual-entry
// option: first when there is nothing to pick, last otherwise. The
// cursor lands on current, else on the first secret for an empty
// current, else on the manual option.
func secretOptions(secrets []string, err error, current string) []huh.Option[string] {
	manual := huh.NewOption(manualSecretLabel, "").Selected(true)
	if err != nil || len(secrets) == 0 {
		return []huh.Option[string]{manual}
	}

	options := lo.Map(secrets, func(s string, i int) huh.Option[string] {
		return huh.NewOption(s, s).Selected(s == current || (current == "" && i == 0))
	})

	return append(options, manual)
}

// secretsDescription explains what the secret picker offers, including
// why discovery came back empty.
func secretsDescription(secrets []string, err error, namespace string) string {
	switch {
	case err != nil:
		return "Could not list secrets: " + err.Error()
	case len(secrets) == 0:
		return "No secrets found in namespace " + namespace
	default:
		return "Pick from existing secrets in namespace " + namespace
	}
}

func newAgentCredsForm(b api.Backend) formSpec {
	return newCredsForm(b, b.AgentCredentials(), "Agent credentials", b.SetAgentCredentials)
}

func newPoolingCredsForm(b api.Backend) formSpec {
	return newCredsForm(b, b.PoolingCredentials(), "Pooling credentials", b.SetPoolingCredentials)
}
