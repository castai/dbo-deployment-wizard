package backend

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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
		"timed out lookup reports the cluster unreachable": {
			err:     fmt.Errorf("helm status: %w", context.DeadlineExceeded),
			want:    false,
			wantErr: `kubernetes cluster unreachable: context "kind-test" did not respond within 10s`,
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

// An unreachable cluster keeps kubectl and helm waiting far longer than
// the UI can; every lookup must carry a deadline.
func TestClusterLookups_AreBounded(t *testing.T) {
	requireBounded := func(t testing.TB, ctx context.Context) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "cluster lookup ran without a deadline")
		require.LessOrEqual(t, time.Until(deadline), clusterQueryTimeout)
	}

	tests := map[string]func(w *Wizard){
		"deployment exists": func(w *Wizard) { _, _ = w.DeploymentExists() },
		"secrets":           func(w *Wizard) { _ = w.Secrets() },
		"installation impact": func(w *Wizard) {
			_, _ = w.SummarizeInstallationImpact()
		},
	}
	for name, lookup := range tests {
		t.Run(name, func(t *testing.T) {
			k := backendmocks.NewMockKubectl(t)
			k.EXPECT().HelmReleaseExists(mock.Anything, "kind-test", "castai-db-optimizer", "castai-dbo").
				RunAndReturn(func(ctx context.Context, _, _, _ string) (bool, error) {
					requireBounded(t, ctx)

					return false, nil
				}).Maybe()
			k.EXPECT().ListSecrets(mock.Anything, "kind-test", "castai-db-optimizer").
				RunAndReturn(func(ctx context.Context, _, _ string) ([]string, error) {
					requireBounded(t, ctx)

					return nil, nil
				}).Maybe()

			lookup(newReleaseLookupWizard(t, k))
			require.NotEmpty(t, k.Calls)
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
		exists          bool
		lookupErr       error
		existingSecrets []string
		agentCreds      api.Credentials
		poolingCreds    api.Credentials
		want            []string
		wantErr         string
	}{
		"new installation creates its secrets": {
			agentCreds:   api.Credentials{Username: "agent", Password: "agentpass"},
			poolingCreds: api.Credentials{Username: "pooler", Password: "poolpass"},
			want: []string{
				"New Helm deployment to be created: castai-db-optimizer/castai-dbo",
				"New api key secret will be created: castai-dbo-api-key",
				"New db agent secret will be created: castai-dbo-agent-credentials",
				"New pooling secret will be created: castai-dbo-pooling-credentials",
			},
		},
		"existing secrets update": {
			agentCreds:      api.Credentials{Username: "agent", Password: "agentpass"},
			poolingCreds:    api.Credentials{Username: "pooler", Password: "poolpass"},
			existingSecrets: []string{"castai-dbo-api-key", "castai-dbo-agent-credentials"},
			want: []string{
				"New Helm deployment to be created: castai-db-optimizer/castai-dbo",
				"Existing api key secret will be updated: castai-dbo-api-key",
				"Existing db agent secret will be updated: castai-dbo-agent-credentials",
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
			if tt.lookupErr == nil {
				k.EXPECT().ListSecrets(mock.Anything, "kind-test", "castai-db-optimizer").
					Return(tt.existingSecrets, nil)
			}

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
