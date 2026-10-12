package message

import (
	"database/sql"
	"encoding/json"
	"time"

	"heckel.io/ntfy/v2/model"
)

const (
	tailPageSize    = 1000        // Rows per query
	tailMaxPages    = 10          // Queries per read at most, so a read drains a backlog of up to 10,000 rows
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

// Next returns the published messages written since the last call, in write order, reading
// pages until it is caught up or has read tailMaxPages. It reads the primary: a replica may lag,
// and what it does not show yet would be skipped as a hole.
func (t *Tail) Next() ([]*model.Message, error) {
	defer t.expireHoles()
	messages := make([]*model.Message, 0)
	for i := 0; i < tailMaxPages; i++ {
		page, full, err := t.nextPage()
		if err != nil {
			if len(messages) > 0 {
				return messages, nil // The pages read so far are done; the failed one is retried next time
			}
			return nil, err
		}
		messages = append(messages, page...)
		if !full {
			break
		}
	}
	return messages, nil
}

// nextPage reads one page of rows: those after the cursor, plus the holes (row ids skipped
// earlier because they were not committed yet). It returns the published messages among them, and
// true if the page was full, so more may be waiting. For each row:
//   - a hole that is now visible is "filled": it is removed from the holes
//   - a row after the cursor moves the cursor to it; the ids it jumped over are "skipped", and
//     become holes
//
// Example: cursor 103, holes {104}; the page returns rows 104, 105 and 108. Row 104 fills its
// hole, 105 moves the cursor, and 108 moves it again, skipping 106 and 107. Afterwards: cursor
// 108, holes {106, 107}. Nothing changes if the read fails part way, so it is retried whole.
func (t *Tail) nextPage() ([]*model.Message, bool, error) {
	holes := make([]int64, 0, len(t.holes))
	for id := range t.holes {
		holes = append(holes, id)
	}
	holesJSON, err := json.Marshal(holes) // One parameter for any number of holes, so the statement text never changes
	if err != nil {
		return nil, false, err
	}
	rows, err := t.cache.db.Primary().Query(t.cache.queries.selectMessagesAfterRowID, t.cursor, string(holesJSON), tailPageSize)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	cursor, count := t.cursor, 0                          // Applied to t only once the whole page was read
	filled, skipped := make([]int64, 0), make([]int64, 0) // Holes now visible; row ids jumped over (new holes)
	messages := make([]*model.Message, 0)                 // Published messages, in row id order
	for rows.Next() {
		m, rowID, published, err := readTailRow(rows)
		if err != nil {
			return nil, false, err
		}
		count++
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
		return nil, false, err
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
	return messages, count == tailPageSize, nil
}

// readTailRow reads a row of selectMessagesAfterRowID: the message, then its row id and whether
// it is published (false for a scheduled message)
func readTailRow(rows *sql.Rows) (*model.Message, int64, bool, error) {
	var rowID int64
	var published bool
	m, err := readMessage(rows, &rowID, &published)
	if err != nil {
		return nil, 0, false, err
	}
	return m, rowID, published, nil
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
