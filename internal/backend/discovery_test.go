package backend

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

func TestCurrentKubeContext(t *testing.T) {
	tests := map[string]struct {
		current string
		err     error
		wantErr string
	}{
		"current context":          {current: "kind-a"},
		"lookup failure is passed": {err: errors.New("kubeconfig missing"), wantErr: "kubeconfig missing"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			k := backendmocks.NewMockKubectl(t)
			k.EXPECT().CurrentKubeContext(mock.Anything).Return(tt.current, tt.err)

			got, err := newReleaseLookupWizard(t, k).CurrentKubeContext()
			r.Equal(tt.current, got)
			if tt.wantErr == "" {
				r.NoError(err)

				return
			}
			r.EqualError(err, tt.wantErr)
		})
	}
}

func TestLatestChartVersion(t *testing.T) {
	tests := map[string]struct {
		chart   string
		want    string
		wantErr string
	}{
		"newest published version": {chart: "castai-dbo", want: "1.4.2"},
		"unknown chart":            {chart: "nope", wantErr: `chart "nope" not found in helm repository`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			w := &Wizard{ctx: t.Context(), repo: newTestClient(t, testHelmIndex), cfg: Config{ChartName: tt.chart}}
			got, err := w.LatestChartVersion()
			r.Equal(tt.want, got)
			if tt.wantErr == "" {
				r.NoError(err)

				return
			}
			r.EqualError(err, tt.wantErr)
		})
	}
}
