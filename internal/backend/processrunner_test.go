package backend

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

func TestRunHelmYAML(t *testing.T) {
	// shellReturning stubs one helm run answering with out.
	shellReturning := func(t *testing.T, out []byte) *backendmocks.MockProcessRunner {
		sh := backendmocks.NewMockProcessRunner(t)
		sh.EXPECT().Run(mock.Anything, "helm", mock.Anything, []string{"get", "manifest"}).
			Return(out, nil)

		return sh
	}

	t.Run("decodes each document", func(t *testing.T) {
		sh := shellReturning(t, []byte(
			"---\nkind: Deployment\nmetadata:\n  name: db-agent\n---\nkind: Service\nmetadata:\n  name: db-agent\n"))

		objects, err := runHelmYAML[[]manifestObject](context.Background(), sh, []string{"get", "manifest"})
		require.NoError(t, err)
		require.Len(t, objects, 2)
		require.Equal(t, "Deployment", objects[0].Kind)
		require.Equal(t, "db-agent", objects[0].Metadata.Name)
		require.Equal(t, "Service", objects[1].Kind)
	})

	t.Run("empty output decodes to none", func(t *testing.T) {
		sh := shellReturning(t, nil)

		objects, err := runHelmYAML[[]manifestObject](context.Background(), sh, []string{"get", "manifest"})
		require.NoError(t, err)
		require.Empty(t, objects)
	})

	t.Run("malformed yaml errors", func(t *testing.T) {
		sh := shellReturning(t, []byte("not yaml: ["))

		_, err := runHelmYAML[[]manifestObject](context.Background(), sh, []string{"get", "manifest"})
		require.ErrorContains(t, err, "parse helm result")
	})
}
