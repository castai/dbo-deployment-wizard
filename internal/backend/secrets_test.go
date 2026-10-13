package backend

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

func TestSecrets(t *testing.T) {
	tests := map[string]struct {
		secrets []string
		err     error
		want    []string
		wantErr string
	}{
		"discovered secrets":        {secrets: []string{"a", "b"}, want: []string{"a", "b"}},
		"empty namespace":           {secrets: nil, want: nil},
		"unreachable cluster fails": {err: errors.New("context unreachable"), wantErr: "context unreachable"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			k := backendmocks.NewMockKubectl(t)
			k.EXPECT().ListSecrets(mock.Anything, "kind-test", "castai-db-optimizer").Return(tt.secrets, tt.err)

			got, err := newReleaseLookupWizard(t, k).Secrets()
			r.Equal(tt.want, got)
			if tt.wantErr == "" {
				r.NoError(err)

				return
			}
			r.EqualError(err, tt.wantErr)
		})
	}
}
