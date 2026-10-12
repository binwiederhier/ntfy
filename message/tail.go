package message

import (
	"database/sql"
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

// Tail returns a tail that starts after the last message written so far. The gaps between the
// last tailPageSize row ids start out as holes: a row id handed out but not committed yet, below
// one that already is, must not be skipped.
func (c *Cache) Tail() (*Tail, error) {
	rows, err := c.db.Primary().Query(c.queries.selectRecentRowIDs, tailPageSize)
	if err != nil {
		return nil, err
	}
	rowIDs, err := readRowIDs(rows)
	if err != nil {
		return nil, err
	}
	t := &Tail{
		cache: c,
		holes: make(map[int64]time.Time),
	}
	now := time.Now()
	for i := len(rowIDs) - 1; i >= 0; i-- { // Ascending
		if t.cursor > 0 {
			t.addHoles(t.cursor, rowIDs[i], now)
		}
		t.cursor = rowIDs[i]
	}
	return t, nil
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
	cursor := t.cursor
	filled, skipped := make([]int64, 0), make([]int64, 0)
	messages := make([]*model.Message, 0)
	for rows.Next() {
		var rowID int64
		var published bool
		m, err := readMessage(rows, &rowID, &published)
		if err != nil {
			return nil, err
		}
		if _, ok := t.holes[rowID]; ok {
			filled = append(filled, rowID)
		} else if rowID > cursor {
			// Row ids between the cursor and this one are not visible (yet)
			for id := cursor + 1; id < rowID && len(skipped) < tailMaxHoles; id++ {
				skipped = append(skipped, id)
			}
			cursor = rowID
		}
		if published { // Scheduled messages go out through the delayed sender
			messages = append(messages, m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Only now: a read that fails part way is retried whole, from the same position
	now := time.Now()
	for _, id := range filled {
		delete(t.holes, id)
	}
	for _, id := range skipped {
		if len(t.holes) >= tailMaxHoles {
			break
		}
		t.holes[id] = now
	}
	t.cursor = cursor
	return messages, nil
}

// addHoles records the row ids between from and to (both exclusive) as holes
func (t *Tail) addHoles(from, to int64, now time.Time) {
	for id := from + 1; id < to && len(t.holes) < tailMaxHoles; id++ {
		t.holes[id] = now
	}
}

func (t *Tail) expireHoles() {
	for id, skipped := range t.holes {
		if time.Since(skipped) > tailHoleTimeout {
			delete(t.holes, id)
		}
	}
}

func readRowIDs(rows *sql.Rows) ([]int64, error) {
	defer rows.Close()
	rowIDs := make([]int64, 0)
	for rows.Next() {
		var rowID int64
		if err := rows.Scan(&rowID); err != nil {
			return nil, err
		}
		rowIDs = append(rowIDs, rowID)
	}
	return rowIDs, rows.Err()
}
