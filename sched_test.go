package sectorstorage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithPriority(t *testing.T) {
	ctx := context.Background()

	require.Equal(t, DefaultSchedPriority, getPriority(ctx))

	ctx = WithPriority(ctx, 2222)

	require.Equal(t, 2222, getPriority(ctx))
}
