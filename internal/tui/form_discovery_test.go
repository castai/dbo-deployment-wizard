package tui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoveryDescription(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"discovery succeeded": {want: "Pick one."},
		"discovery failed":    {err: errors.New("kubeconfig missing"), want: "Could not list kubectl contexts: kubeconfig missing"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tt.want, discoveryDescription(tt.err, "kubectl contexts", "Pick one."))
		})
	}
}

func TestDiscoveryFailureShownInForm(t *testing.T) {
	tests := map[string]struct {
		to    screenID
		setup func(b *fakeBackend)
		want  string
	}{
		"kube contexts": {
			to:    screenKubeContext,
			setup: func(b *fakeBackend) { b.kubeContextsErr = errors.New("kubeconfig missing") },
			want:  "Could not list kubectl contexts: kubeconfig missing",
		},
		"chart versions": {
			to:    screenChartVersion,
			setup: func(b *fakeBackend) { b.versionsErr = errors.New("repo unreachable") },
			want:  "Could not list chart versions: repo unreachable",
		},
		"kube contexts listed": {
			to:    screenKubeContext,
			setup: func(*fakeBackend) {},
			want:  "Pick a Kubernetes cluster to install the Helm chart into.",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			b := newFakeBackend()
			tt.setup(b)
			h := newHarness(t, b)

			h.enterSection(tt.to)

			r.Contains(h.m.View().Content, tt.want)
		})
	}
}
