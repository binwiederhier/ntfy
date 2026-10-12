package message

import (
	"encoding/json"
	"time"

	"heckel.io/ntfy/v2/model"
)

const (
	tailPageSize    = 1000        // Rows per read; a backlog drains at this many per read
	tailHoleTimeout = time.Minute // How long a skipped row id is re-checked before it counts as never committed (rolled back)
	tailMaxHoles    = 10000       // Bounds the skipped row ids re-checked on every call
)

// Tail reads the messages written to the cache in write order (by row id), so a node can find
// what it never received live. Row ids are handed out on insert but only become visible on commit,
// and concurrent writers (cluster nodes) commit out of order: a row id the tail had to skip is a
// hole, and is re-checked until its row shows up or the hole times out.
type Tail struct {
	cache  *Cache
	cursor int64               // Highest row id read so far
	holes  map[int64]time.Time // Skipped row ids below the cursor, and when they were skipped
}

// Tail returns a tail that starts after the last message written so far
func (c *Cache) Tail() (*Tail, error) {
	var cursor int64
	if err := c.db.Primary().QueryRow(c.queries.selectMaxRowID).Scan(&cursor); err != nil {
		return nil, err
	}
	return &Tail{
		cache:  c,
		cursor: cursor,
		holes:  make(map[int64]time.Time),
	}, nil
}

// Next returns the published messages written since the last call, in write order, at most
// tailPageSize rows per call. It reads the primary: a replica may lag, and what it does not show
// yet would be skipped as a hole.
func (t *Tail) Next() ([]*model.Message, error) {
	defer t.expireHoles()
	holes := make([]int64, 0, len(t.holes))
	for id := range t.holes {
		holes = append(holes, id)
	}
	holesJSON, err := json.Marshal(holes)
	if err != nil {
		return nil, err
	}
	rows, err := t.cache.db.Primary().Query(t.cache.queries.selectMessagesAfterRowID, t.cursor, string(holesJSON), tailPageSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now()
	messages := make([]*model.Message, 0)
	for rows.Next() {
		var rowID int64
		var published bool
		m, err := readMessage(rows, &rowID, &published)
		if err != nil {
			return nil, err
		}
		if _, ok := t.holes[rowID]; ok {
			delete(t.holes, rowID)
		} else if rowID > t.cursor {
			// Row ids between the cursor and this one are not visible (yet)
			for id := t.cursor + 1; id < rowID && len(t.holes) < tailMaxHoles; id++ {
				t.holes[id] = now
			}
			t.cursor = rowID
		}
		if published { // Scheduled messages go out through the delayed sender
			messages = append(messages, m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func (t *Tail) expireHoles() {
	for id, skipped := range t.holes {
		if time.Since(skipped) > tailHoleTimeout {
			delete(t.holes, id)
		}
	}
}
