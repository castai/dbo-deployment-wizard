package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

func TestReconcileComponents(t *testing.T) {
	tests := map[string]struct {
		sel  []string
		want []string
	}{
		"pooling with db-proxy kept": {
			sel:  []string{api.ComponentDBProxy, api.ComponentPooling},
			want: []string{api.ComponentDBProxy, api.ComponentPooling},
		},
		"pooling without db-proxy dropped": {
			sel:  []string{api.ComponentDBAgent, api.ComponentPooling},
			want: []string{api.ComponentDBAgent},
		},
		"no pooling untouched": {
			sel:  []string{api.ComponentDBAgent},
			want: []string{api.ComponentDBAgent},
		},
		"empty": {
			sel:  []string{},
			want: []string{},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tt.want, reconcileComponents(tt.sel))
		})
	}
}

func TestReconcileComponents_DoesNotMutateInput(t *testing.T) {
	r := require.New(t)
	sel := []string{api.ComponentDBAgent, api.ComponentPooling}

	_ = reconcileComponents(sel)

	r.Equal([]string{api.ComponentDBAgent, api.ComponentPooling}, sel)
}

func TestComponentName(t *testing.T) {
	tests := map[string]struct {
		component string
		want      string
	}{
		"db-agent": {component: api.ComponentDBAgent, want: "Performance Advisor"},
		"db-proxy": {component: api.ComponentDBProxy, want: "Database Proxy"},
		"pooling":  {component: api.ComponentPooling, want: "Connection Pooling"},
		"unknown":  {component: "db-optimizer", want: "db-optimizer"},
		"empty":    {component: "", want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tt.want, componentName(tt.component))
		})
	}
	r := require.New(t)
	for _, c := range api.AllComponents {
		r.Contains(componentNames, c, "component %q has no display name", c)
	}
}

func TestComponentsShownByDisplayName(t *testing.T) {
	tests := map[string]struct {
		components []string
		wantReview []string
		wantForm   []string
	}{
		"all selected": {
			components: api.AllComponents,
			wantReview: []string{
				"Components: Performance Advisor, Database Proxy, Connection Pooling",
				"Performance Advisor credentials:",
				"Connection Pooling credentials:",
			},
			wantForm: []string{"Performance Advisor", "Database Proxy", "Connection Pooling", "Connection Pooling requires Database Proxy"},
		},
		"none selected is marked required": {
			components: nil,
			wantReview: []string{"Components: " + unsetPlaceholder, requiredSub},
			wantForm:   []string{"Performance Advisor"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			b := newFakeBackend()
			b.components = tt.components
			h := newHarness(t, b)

			review := h.m.View().Content
			for _, want := range tt.wantReview {
				r.Contains(review, want)
			}
			r.NotContains(review, "db-agent")

			h.enterSection(screenComponents)
			form := h.m.View().Content
			for _, want := range tt.wantForm {
				r.Contains(form, want)
			}
			r.NotContains(form, "db-proxy")
		})
	}
}
