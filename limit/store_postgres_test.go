package limit

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestStore_MayHaveCommittedPostgres(t *testing.T) {
	// A timeout may have committed (the answer came after the deadline); a server error or an
	// error before sending never did. A timeout waiting for a pool connection is not told apart
	// from one after sending, so it counts as written too (counts may be lost, never doubled).
	require.True(t, mayHaveCommittedPostgres(context.DeadlineExceeded))
	require.False(t, mayHaveCommittedPostgres(&pgconn.PgError{Code: "40001"}))
	require.False(t, mayHaveCommittedPostgres(errors.New("connection refused")))
}
