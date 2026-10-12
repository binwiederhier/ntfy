package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func TestVisitor_UserStatsDeltaFlush(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthStatsQueueWriterInterval = 300 * time.Millisecond
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "test",
			MessageLimit: 100,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "test"))

		// Publish 3 messages; the visitor enqueues a stats delta after each publish
		for i := 0; i < 3; i++ {
			response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
				"Authorization": util.BasicAuth("phil", "phil"),
			})
			require.Equal(t, 200, response.Code)
		}

		// After two flush intervals, the persisted count must be exactly 3: per-publish
		// enqueues must be deltas, not absolute snapshots (snapshots would add up to 1+2+3=6)
		time.Sleep(time.Second)
		u, err := s.userManager.User("phil")
		require.Nil(t, err)
		require.Equal(t, int64(3), u.Stats.Messages)
	})
}
