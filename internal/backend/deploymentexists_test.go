package backend

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

// newReleaseLookupWizard builds a Wizard around a kubectl mock with a
// fixed release-lookup configuration.
func newReleaseLookupWizard(k Kubectl) *Wizard {
	return &Wizard{
		ctx: context.Background(),
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

			w := newReleaseLookupWizard(k)
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

	w := newReleaseLookupWizard(k)

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
