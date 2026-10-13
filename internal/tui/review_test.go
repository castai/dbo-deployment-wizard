package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

func TestMaskPassword(t *testing.T) {
	tests := map[string]struct {
		password string
		want     int
	}{
		"short password shows its length": {password: "hunter2", want: 7},
		"long password capped":            {password: strings.Repeat("x", 40), want: maskedPasswordMax},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(strings.Repeat("•", tt.want), maskPassword(tt.password))
		})
	}
}

func TestCoordinateSub(t *testing.T) {
	tests := map[string]struct {
		value       string
		discovering bool
		err         error
		want        string
		wantWarn    bool
	}{
		"set value has no sub":            {value: "kind-a"},
		"set value hides a failure":       {value: "kind-a", err: errors.New("boom")},
		"unset while discovering":         {discovering: true},
		"unset after a failure":           {err: errors.New("kubeconfig missing"), want: "⚠ kubeconfig missing", wantWarn: true},
		"unset after a clean discovery":   {want: requiredSub, wantWarn: true},
		"failure is truncated to one row": {err: errors.New(strings.Repeat("x", 200)), want: "⚠ " + strings.Repeat("x", 99) + "…", wantWarn: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			sub, warn := coordinateSub(tt.value, tt.discovering, tt.err)
			r.Equal(tt.want, sub)
			r.Equal(tt.wantWarn, warn)
		})
	}
}

func TestReview_DiscoversDefaults(t *testing.T) {
	// creds satisfy the credential rows, so the only "required" marker
	// left could come from a coordinate row.
	creds := api.Credentials{SecretName: "db-creds"}
	tests := map[string]struct {
		setup       func(b *fakeBackend)
		wantContext string
		wantVersion string
		wantView    []string
		notInView   []string
	}{
		"unset coordinates take the detected defaults": {
			setup: func(b *fakeBackend) {
				b.kubeContext, b.chartVersion, b.currentContext = "", "", "kind-z"
				b.agentCreds, b.poolingCreds = creds, creds
			},
			wantContext: "kind-z",
			wantVersion: "1.1.0",
			wantView:    []string{"kind-z", "1.1.0 (latest)", "Installing new Helm deployment: castai-dbo"},
			notInView:   []string{requiredSub},
		},
		"preset coordinates are kept": {
			setup: func(b *fakeBackend) {
				b.currentContext = "kind-z"
				b.agentCreds, b.poolingCreds = creds, creds
			},
			wantContext: "kind-a",
			wantVersion: "1.0.0",
			wantView:    []string{"kind-a", "1.0.0"},
			notInView:   []string{"kind-z", "(latest)", requiredSub},
		},
		"detection failures show on the rows and skip the release lookup": {
			setup: func(b *fakeBackend) {
				b.kubeContext, b.chartVersion = "", ""
				b.currentContextErr = errors.New("kubeconfig missing")
				b.latestErr = errors.New("repo unreachable")
				b.existsErr = errors.New("release lookup must not run without a context")
			},
			wantView:  []string{"⚠ kubeconfig missing", "⚠ repo unreachable", "Helm deployment: castai-dbo"},
			notInView: []string{"release lookup must not run", reviewTitleLoading},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			b := newFakeBackend()
			tt.setup(b)
			h := newHarness(t, b)

			r.False(h.m.review.defaults.loading())
			r.Equal(tt.wantContext, b.kubeContext)
			r.Equal(tt.wantVersion, b.chartVersion)
			view := h.m.View().Content
			for _, want := range tt.wantView {
				r.Contains(view, want)
			}
			for _, unwanted := range tt.notInView {
				r.NotContains(view, unwanted)
			}
		})
	}
}

// TestReview_DiscoveryRunsOnce: returning from a section re-checks the
// release only; a context picked in the meantime is not overwritten.
func TestReview_DiscoveryRunsOnce(t *testing.T) {
	r := require.New(t)
	b := newFakeBackend()
	b.kubeContext, b.currentContext = "", "kind-z"
	h := newHarness(t, b)
	r.Equal("kind-z", b.kubeContext)

	b.currentContext = "kind-other"
	h.enterSection(screenKubeContext)
	h.press(tea.KeyDown)
	h.press(tea.KeyEnter)

	r.True(h.onReview())
	r.Equal("kind-b", b.kubeContext)
	r.Contains(h.m.View().Content, "Installing new Helm deployment: castai-dbo")
}

// TestReview_DiscoveryLandsWhileSectionOpen: a section opened before
// discovery answers must not lose the answer — the hub keeps
// receiving background results while a section is active.
func TestReview_DiscoveryLandsWhileSectionOpen(t *testing.T) {
	r := require.New(t)
	b := newFakeBackend()
	b.kubeContext, b.chartVersion, b.currentContext = "", "", "kind-z"
	b.discoveryGate = make(chan struct{})
	t.Cleanup(func() { close(b.discoveryGate) })
	h := newHarness(t, b)
	r.True(h.m.review.defaults.loading())

	h.enterSection(screenNamespace)
	r.IsType(&formScreen{}, h.m.active)
	h.send(asyncResultMsg[discovery]{
		value: discovery{kubeContext: "kind-z", latestVersion: "1.1.0"},
		fetch: h.m.review.defaults.fetch,
	})
	r.False(h.m.review.defaults.loading())
	r.Equal("kind-z", b.kubeContext)
	r.Equal("1.1.0", b.chartVersion)

	h.press(tea.KeyEsc)
	r.True(h.onReview())
	view := h.m.View().Content
	r.NotContains(view, reviewTitleDiscovering)
	r.Contains(view, "Installing new Helm deployment: castai-dbo")
	r.Contains(view, "1.1.0 (latest)")
}
