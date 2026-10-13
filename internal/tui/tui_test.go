package tui

import (
	"errors"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

func TestNavigation_SectionsOpenAndEscReturnsWithoutCommitting(t *testing.T) {
	tests := map[string]struct {
		to screenID
	}{
		"kube context":        {to: screenKubeContext},
		"namespace":           {to: screenNamespace},
		"chart version":       {to: screenChartVersion},
		"agent credentials":   {to: screenAgentCreds},
		"components":          {to: screenComponents},
		"pooling credentials": {to: screenPoolingCreds},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			b := newFakeBackend()
			want := *b
			h := newHarness(t, b)

			h.enterSection(tt.to)
			r.IsType(&formScreen{}, h.m.active)

			h.typeText("zz")
			h.press(tea.KeyEsc)
			r.True(h.onReview())
			r.False(h.quit)
			r.Equal(want, *b)
		})
	}
}

func TestNavigation_FormCompletionCommits(t *testing.T) {
	tests := map[string]struct {
		to    screenID
		keys  func(h *harness)
		check func(r *require.Assertions, b *fakeBackend)
	}{
		"namespace, ctrl+u edits instead of aborting": {
			to: screenNamespace,
			keys: func(h *harness) {
				h.ctrl('u')
				h.typeText("prod")
				h.press(tea.KeyEnter)
			},
			check: func(r *require.Assertions, b *fakeBackend) { r.Equal("prod", b.namespace) },
		},
		"kube context": {
			to: screenKubeContext,
			keys: func(h *harness) {
				h.press(tea.KeyDown)
				h.press(tea.KeyEnter)
			},
			check: func(r *require.Assertions, b *fakeBackend) { r.Equal("kind-b", b.kubeContext) },
		},
		"chart version": {
			to: screenChartVersion,
			keys: func(h *harness) {
				h.press(tea.KeyUp)
				h.press(tea.KeyEnter)
			},
			check: func(r *require.Assertions, b *fakeBackend) { r.Equal("1.1.0", b.chartVersion) },
		},
		"components drop pooling with db-proxy": {
			to: screenComponents,
			keys: func(h *harness) {
				h.press(tea.KeyDown)
				h.typeText("x")
				h.press(tea.KeyEnter)
			},
			check: func(r *require.Assertions, b *fakeBackend) { r.Equal([]string{api.ComponentDBAgent}, b.components) },
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			b := newFakeBackend()
			h := newHarness(t, b)

			h.enterSection(tt.to)
			tt.keys(h)

			r.True(h.onReview())
			r.False(h.quit)
			tt.check(r, b)
		})
	}
}

func TestNavigation_CommitErrorShowsOnReview(t *testing.T) {
	r := require.New(t)
	b := newFakeBackend()
	b.setComponentsErr = errors.New("pooling requires db-proxy")
	h := newHarness(t, b)

	h.enterSection(screenComponents)
	h.press(tea.KeyEnter)

	r.True(h.onReview())
	r.Equal("pooling requires db-proxy", h.m.review.reviewError)
	r.Contains(h.m.View().Content, "pooling requires db-proxy")
}

func TestNavigation_Continue(t *testing.T) {
	tests := map[string]struct {
		validateErr error
		wantConfirm bool
	}{
		"valid config opens confirm": {wantConfirm: true},
		"validation failure stays":   {validateErr: errors.New("agent credentials are required")},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			b := newFakeBackend()
			b.validateErr = tt.validateErr
			h := newHarness(t, b)

			h.enterSection(screenConfirm)

			if !tt.wantConfirm {
				r.True(h.onReview())
				r.Equal(tt.validateErr.Error(), h.m.review.reviewError)
				r.Contains(h.m.View().Content, tt.validateErr.Error())

				return
			}
			r.IsType(&confirmModel{}, h.m.active)
			r.Contains(h.m.View().Content, "install castai-dbo")

			h.press(tea.KeyEsc)
			r.True(h.onReview())
			r.False(h.quit)
		})
	}
}

func TestConfirm_Apply(t *testing.T) {
	tests := map[string]struct {
		summaryErr error
		wantDone   bool
	}{
		"loaded summary applies":   {wantDone: true},
		"failed summary blocks it": {summaryErr: errors.New("cluster unreachable")},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			b := newFakeBackend()
			b.summaryErr = tt.summaryErr
			h := newHarness(t, b)

			h.enterSection(screenConfirm)
			h.press(tea.KeyEnter)

			r.Equal(tt.wantDone, h.m.done)
			r.Equal(tt.wantDone, h.quit)
			if tt.summaryErr != nil {
				r.Contains(h.m.View().Content, tt.summaryErr.Error())
			}
		})
	}
}

func TestQuitKeys(t *testing.T) {
	tests := map[string]struct {
		from screenID
		key  tea.KeyPressMsg
	}{
		"esc on review":     {from: screenReview, key: tea.KeyPressMsg{Code: tea.KeyEsc}},
		"ctrl+c on review":  {from: screenReview, key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
		"ctrl+c on a form":  {from: screenNamespace, key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
		"ctrl+c on confirm": {from: screenConfirm, key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			h := newHarness(t, newFakeBackend())
			if tt.from != screenReview {
				h.enterSection(tt.from)
			}

			h.send(tt.key)

			r.True(h.quit)
			r.True(h.m.cancel)
			r.False(h.m.done)
		})
	}
}

func TestEscInFilteredSelect(t *testing.T) {
	r := require.New(t)
	h := newHarness(t, newFakeBackend())
	h.enterSection(screenKubeContext)
	sel := func() *huh.Select[string] {
		s, ok := h.m.active.(*formScreen).form.GetFocusedField().(*huh.Select[string])
		r.True(ok)

		return s
	}

	h.typeText("/b")
	r.True(sel().GetFiltering())

	// huh's first Esc sets the filter, the second clears it.
	h.press(tea.KeyEsc)
	r.False(sel().GetFiltering())
	r.IsType(&formScreen{}, h.m.active)
	h.press(tea.KeyEsc)
	r.IsType(&formScreen{}, h.m.active)

	h.press(tea.KeyEsc)
	r.True(h.onReview())
	r.False(h.quit)
}

func TestReview_DeploymentLookupError(t *testing.T) {
	r := require.New(t)
	b := newFakeBackend()
	b.existsErr = errors.New("helm: cluster unreachable")
	h := newHarness(t, b)

	r.Equal(b.existsErr, h.m.review.release.err)
	r.Contains(h.m.View().Content, "helm: cluster unreachable")
}

func TestReview_SlashDoesNotFilter(t *testing.T) {
	r := require.New(t)
	h := newHarness(t, newFakeBackend())

	h.typeText("/")

	r.Equal(list.Unfiltered, h.m.review.list.FilterState())
}
