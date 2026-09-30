package backend

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

// newReleaseLookupWizard builds a Wizard around a kubectl mock with a
// fixed release-lookup configuration.
func newReleaseLookupWizard(t testing.TB, k Kubectl) *Wizard {
	return &Wizard{
		ctx: t.Context(),
		k:   k,
		cfg: Config{
			ReleaseName: "castai-dbo",
			Namespace:   "castai-db-optimizer",
			KubeContext: "kind-test",
		},
	}
}

func TestDeploymentExists(t *testing.T) {
	tests := map[string]struct {
		exists  bool
		err     error
		want    bool
		wantErr string
	}{
		"no release is a new installation": {exists: false, want: false},
		"existing release is an upgrade":   {exists: true, want: true},
		"lookup failure returns the error": {
			err:     errors.New("context unreachable"),
			want:    false,
			wantErr: "context unreachable",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			k := backendmocks.NewMockKubectl(t)
			k.EXPECT().HelmReleaseExists(mock.Anything, "kind-test", "castai-db-optimizer", "castai-dbo").
				Return(tt.exists, tt.err)

			w := newReleaseLookupWizard(t, k)
			got, err := w.DeploymentExists()
			r.Equal(tt.want, got)
			if tt.wantErr == "" {
				r.NoError(err)

				return
			}
			r.EqualError(err, tt.wantErr)
		})
	}
}

func TestDeploymentExists_UsesCurrentCoordinates(t *testing.T) {
	r := require.New(t)

	k := backendmocks.NewMockKubectl(t)
	k.EXPECT().HelmReleaseExists(mock.Anything, "kind-test", "castai-db-optimizer", "castai-dbo").
		Return(true, nil)
	k.EXPECT().HelmReleaseExists(mock.Anything, "kind-test", "other-ns", "castai-dbo").
		Return(false, nil)
	k.EXPECT().HelmReleaseExists(mock.Anything, "other-ctx", "other-ns", "castai-dbo").
		Return(true, nil)

	w := newReleaseLookupWizard(t, k)

	exists, err := w.DeploymentExists()
	r.True(exists)
	r.NoError(err)

	w.SetNamespace("other-ns")
	exists, err = w.DeploymentExists()
	r.False(exists)
	r.NoError(err)

	w.SetKubeContext("other-ctx")
	exists, err = w.DeploymentExists()
	r.True(exists)
	r.NoError(err)
}

func TestSummarizeInstallationImpact(t *testing.T) {
	base := Config{
		ReleaseName: "castai-dbo",
		Namespace:   "castai-db-optimizer",
		KubeContext: "kind-test",
	}

	tests := map[string]struct {
		exists       bool
		lookupErr    error
		agentCreds   api.Credentials
		poolingCreds api.Credentials
		want         []string
		wantErr      string
	}{
		"new installation with password pairs": {
			agentCreds:   api.Credentials{Username: "agent", Password: "agentpass"},
			poolingCreds: api.Credentials{Username: "pooler", Password: "poolpass"},
			want: []string{
				"New Helm deployment to be created: castai-db-optimizer/castai-dbo",
				// TODO: this is wrong? we need to check what actually exists in the cluster
				"New api key secret will be created: castai-dbo-api-key",
				"New db agent secret will be created: castai-dbo-agent-credentials",
				"New pooling secret will be created: castai-dbo-pooling-credentials",
			},
		},
		"upgrade": {
			exists: true,
			want: []string{
				"Existing Helm deployment will be updated: castai-db-optimizer/castai-dbo",
				"New api key secret will be created: castai-dbo-api-key",
			},
		},
		"lookup failure returns the error": {
			lookupErr: errors.New("context unreachable"),
			want:      nil,
			wantErr:   "context unreachable",
		},
		"existing secret references create nothing": {
			agentCreds: api.Credentials{SecretName: "agent-secret"},
			want: []string{
				"New Helm deployment to be created: castai-db-optimizer/castai-dbo",
				"New api key secret will be created: castai-dbo-api-key",
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			k := backendmocks.NewMockKubectl(t)
			k.EXPECT().HelmReleaseExists(mock.Anything, "kind-test", "castai-db-optimizer", "castai-dbo").
				Return(tt.exists, tt.lookupErr)

			cfg := base
			cfg.AgentCreds = tt.agentCreds
			cfg.PoolingCreds = tt.poolingCreds
			w := &Wizard{ctx: t.Context(), k: k, cfg: cfg}

			got, err := w.SummarizeInstallationImpact()
			require.Equal(t, tt.want, got)
			if tt.wantErr == "" {
				require.NoError(t, err)

				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}
