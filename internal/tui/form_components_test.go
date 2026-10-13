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
