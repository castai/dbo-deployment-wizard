package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

type optionView struct {
	label    string
	value    string
	selected bool
}

func TestSecretOptions(t *testing.T) {
	manual := func(selected bool) optionView { return optionView{label: manualSecretLabel, selected: selected} }
	tests := map[string]struct {
		secrets []string
		err     error
		current string
		want    []optionView
	}{
		"discovery failed": {
			err:     errors.New("unreachable"),
			current: "db-creds",
			want:    []optionView{manual(true)},
		},
		"nothing found": {
			want: []optionView{manual(true)},
		},
		"empty current lands on first secret": {
			secrets: []string{"a", "b"},
			want:    []optionView{{"a", "a", true}, {"b", "b", false}, manual(true)},
		},
		"current secret selected": {
			secrets: []string{"a", "b"},
			current: "b",
			want:    []optionView{{"a", "a", false}, {"b", "b", true}, manual(true)},
		},
		"unknown current falls back to manual": {
			secrets: []string{"a"},
			current: "gone",
			want:    []optionView{{"a", "a", false}, manual(true)},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			got := lo.Map(secretOptions(tt.secrets, tt.err, tt.current), func(o huh.Option[string], _ int) optionView {
				// huh keeps the selected flag unexported, so compare
				// against a selected copy.
				return optionView{label: o.Key, value: o.Value, selected: o == huh.NewOption(o.Key, o.Value).Selected(true)}
			})
			r.Equal(tt.want, got)
		})
	}
}

func TestSecretsDescription(t *testing.T) {
	tests := map[string]struct {
		secrets []string
		err     error
		want    string
	}{
		"discovery failed": {err: errors.New("context unreachable"), want: "Could not list secrets: context unreachable"},
		"nothing found":    {want: "No secrets found in namespace ns"},
		"secrets found":    {secrets: []string{"a"}, want: "Pick from existing secrets in namespace ns"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tt.want, secretsDescription(tt.secrets, tt.err, "ns"))
		})
	}
}

func TestCredsForMode(t *testing.T) {
	both := api.Credentials{SecretName: "s", Username: "u", Password: "p"}
	tests := map[string]struct {
		mode string
		want api.Credentials
	}{
		"secret mode drops the pair": {mode: credsModeSecret, want: api.Credentials{SecretName: "s"}},
		"user mode drops the secret": {mode: credsModeUser, want: api.Credentials{Username: "u", Password: "p"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tt.want, credsForMode(both, tt.mode))
		})
	}
}

func TestCredentialsForm(t *testing.T) {
	tests := map[string]struct {
		initial    api.Credentials
		secrets    []string
		secretsErr error
		keys       func(h *harness)
		want       api.Credentials
	}{
		"discovery failure reveals manual input": {
			secretsErr: errors.New("unreachable"),
			keys: func(h *harness) {
				h.press(tea.KeyEnter) // existing secret
				h.press(tea.KeyEnter) // manual option
				h.typeText("typed")
				h.press(tea.KeyEnter)
			},
			want: api.Credentials{SecretName: "typed"},
		},
		"picking a discovered secret clears the pair": {
			initial: api.Credentials{Username: "u"},
			secrets: []string{"a", "b"},
			keys: func(h *harness) {
				h.press(tea.KeyUp) // existing secret
				h.press(tea.KeyEnter)
				h.press(tea.KeyDown)
				h.press(tea.KeyEnter)
			},
			want: api.Credentials{SecretName: "b"},
		},
		"manual option among discovered secrets": {
			secrets: []string{"a"},
			keys: func(h *harness) {
				h.press(tea.KeyEnter)
				h.press(tea.KeyDown)
				h.press(tea.KeyEnter)
				h.typeText("other")
				h.press(tea.KeyEnter)
			},
			want: api.Credentials{SecretName: "other"},
		},
		"username and password clear the secret": {
			initial: api.Credentials{SecretName: "s"},
			secrets: []string{"s"},
			keys: func(h *harness) {
				h.press(tea.KeyDown)
				h.press(tea.KeyEnter)
				h.typeText("u")
				h.press(tea.KeyEnter)
				h.typeText("p")
				h.press(tea.KeyEnter)
			},
			want: api.Credentials{Username: "u", Password: "p"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			b := newFakeBackend()
			b.agentCreds = tt.initial
			b.secrets, b.secretsErr = tt.secrets, tt.secretsErr
			h := newHarness(t, b)

			h.enterSection(screenAgentCreds)
			tt.keys(h)

			r.True(h.onReview())
			r.Equal(tt.want, b.agentCreds)
		})
	}
}

func TestCredentialsForm_DescribesDiscoveryFailure(t *testing.T) {
	r := require.New(t)
	b := newFakeBackend()
	b.secretsErr = errors.New("context unreachable")
	h := newHarness(t, b)

	h.enterSection(screenAgentCreds)
	h.press(tea.KeyEnter)

	r.Contains(h.m.View().Content, "Could not list secrets: context unreachable")
}
