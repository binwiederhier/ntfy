package attachment

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/s3"
	"heckel.io/ntfy/v2/util"
)

const (
	tagStore     = "attachment_store"
	syncInterval = 15 * time.Minute // How often to run the background sync loop
)

var errInvalidFileID = errors.New("invalid file ID")

// Store manages attachment storage with shared logic for size tracking, limiting,
// ID validation, and background sync to reconcile storage with the database.
type Store struct {
	backend              backend
	limit                int64                            // Defined limit of the store in bytes
	size                 int64                            // Current size of the store in bytes
	sizes                map[string]int64                 // File ID -> size, for subtracting on Remove
	attachmentsWithSizes func() (map[string]int64, error) // Returns file ID -> size for active attachments
	orphanGracePeriod    time.Duration                    // Don't delete orphaned objects younger than this
	closeChan            chan struct{}
	doneChan             chan struct{}
	mu                   sync.RWMutex // Protects size and sizes
}

// NewFileStore creates a new file-system backed attachment cache
func NewFileStore(dir string, totalSizeLimit int64, orphanGracePeriod time.Duration, attachmentsWithSizes func() (map[string]int64, error)) (*Store, error) {
	b, err := newFileBackend(dir)
	if err != nil {
		return nil, err
	}
	return newStore(b, totalSizeLimit, orphanGracePeriod, attachmentsWithSizes)
}

// NewS3Store creates a new S3-backed attachment cache. The s3URL must be in the format:
//
//	s3://ACCESS_KEY:SECRET_KEY@BUCKET[/PREFIX]?region=REGION[&endpoint=ENDPOINT][&disable_http2=true]
func NewS3Store(s3URL string, totalSizeLimit int64, orphanGracePeriod time.Duration, attachmentsWithSizes func() (map[string]int64, error)) (*Store, error) {
	config, err := s3.ParseURL(s3URL)
	if err != nil {
		return nil, err
	}
	return newStore(newS3Backend(s3.New(config)), totalSizeLimit, orphanGracePeriod, attachmentsWithSizes)
}

func newStore(backend backend, totalSizeLimit int64, orphanGracePeriod time.Duration, attachmentsWithSizes func() (map[string]int64, error)) (*Store, error) {
	c := &Store{
		backend:              backend,
		limit:                totalSizeLimit,
		sizes:                make(map[string]int64),
		attachmentsWithSizes: attachmentsWithSizes,
		orphanGracePeriod:    orphanGracePeriod,
		closeChan:            make(chan struct{}),
		doneChan:             make(chan struct{}),
	}
	// Hydrate sizes from the database immediately so that Size()/Remaining()/Remove()
	// are accurate from the start, without waiting for the first sync() call.
	if attachmentsWithSizes != nil {
		attachments, err := attachmentsWithSizes()
		if err != nil {
			return nil, fmt.Errorf("attachment store: failed to load existing attachments: %w", err)
		}
		for id, size := range attachments {
			c.sizes[id] = size
			c.size += size
		}
		go c.syncLoop()
	} else {
		close(c.doneChan)
	}
	return c, nil
}

// Write stores an attachment file. The id is validated, and the write is subject to
// the total size limit and any additional limiters. The untrustedLength is a hint
// from the client's Content-Length header; backends may use it to optimize uploads (e.g.
// streaming directly to S3 without buffering).
func (c *Store) Write(id string, reader io.Reader, untrustedLength int64, limiters ...util.Limiter) (int64, error) {
	if !model.ValidMessageID(id) {
		return 0, errInvalidFileID
	}
	log.Tag(tagStore).Field("message_id", id).Debug("Writing attachment")
	limiters = append(limiters, util.NewFixedLimiter(c.Remaining()))
	countingReader := util.NewCountingReader(reader)
	limitReader := util.NewLimitReader(countingReader, limiters...)
	if err := c.backend.Put(id, limitReader, untrustedLength); err != nil {
		c.backend.Delete(id) //nolint:errcheck
		return 0, err
	}
	size := countingReader.Total()
	c.mu.Lock()
	c.size += size
	c.sizes[id] = size
	c.mu.Unlock()
	return size, nil
}

// Read retrieves an attachment file by ID
func (c *Store) Read(id string) (io.ReadCloser, int64, error) {
	if !model.ValidMessageID(id) {
		return nil, 0, errInvalidFileID
	}
	return c.backend.Get(id)
}

// Remove deletes attachment files by ID and subtracts their known sizes from
// the total. Sizes for objects not tracked (e.g. written before this process
// started and before the first sync) are corrected by the next sync() call.
func (c *Store) Remove(ids ...string) error {
	for _, id := range ids {
		if !model.ValidMessageID(id) {
			return errInvalidFileID
		}
	}
	// Remove from backend
	for _, id := range ids {
		log.Tag(tagStore).Field("message_id", id).Debug("Removing attachment")
	}
	if err := c.backend.Delete(ids...); err != nil {
		return err
	}
	// Update total cache size
	c.mu.Lock()
	for _, id := range ids {
		if size, ok := c.sizes[id]; ok {
			c.size -= size
			delete(c.sizes, id)
		}
	}
	if c.size < 0 {
		c.size = 0
	}
	c.mu.Unlock()
	return nil
}

// Sync triggers an immediate reconciliation of storage with the database.
func (c *Store) Sync() error {
	return c.sync()
}

// sync reconciles the backend storage with the database. It lists all objects,
// deletes orphans (not in the valid ID set and older than the grace period), and
// recomputes the total size from the existing attachments in the database.
func (c *Store) sync() error {
	if c.attachmentsWithSizes == nil {
		return nil
	}
	attachmentsWithSizes, err := c.attachmentsWithSizes()
	if err != nil {
		return fmt.Errorf("attachment sync: failed to get existing attachments: %w", err)
	}
	remoteObjects, err := c.backend.List()
	if err != nil {
		return fmt.Errorf("attachment sync: failed to list objects: %w", err)
	}
	// Calculate total cache size and collect orphaned attachments, excluding objects younger
	// than the grace period to account for races, and skipping objects with invalid IDs.
	cutoff := time.Now().Add(-c.orphanGracePeriod)
	var orphanIDs []string
	var count, totalSize int64
	sizes := make(map[string]int64, len(remoteObjects))
	for _, obj := range remoteObjects {
		if !model.ValidMessageID(obj.ID) {
			continue
		}
		if _, ok := attachmentsWithSizes[obj.ID]; !ok && obj.LastModified.Before(cutoff) {
			orphanIDs = append(orphanIDs, obj.ID)
		} else {
			count++
			totalSize += attachmentsWithSizes[obj.ID]
			sizes[obj.ID] = attachmentsWithSizes[obj.ID]
		}
	}
	log.Tag(tagStore).Debug("Attachment store updated: %d attachment(s), %s", count, util.FormatSizeHuman(totalSize))
	c.mu.Lock()
	c.size = totalSize
	c.sizes = sizes
	c.mu.Unlock()
	// Delete orphaned attachments
	if len(orphanIDs) > 0 {
		log.Tag(tagStore).Debug("Deleting %d orphaned attachment(s)", len(orphanIDs))
		if err := c.backend.Delete(orphanIDs...); err != nil {
			return fmt.Errorf("attachment sync: failed to delete orphaned objects: %w", err)
		}
	}
	// Clean up incomplete uploads (S3 only)
	if err := c.backend.DeleteIncomplete(cutoff); err != nil {
		log.Tag(tagStore).Err(err).Warn("Failed to abort incomplete uploads from attachment cache")
	}
	return nil
}

// Size returns the current total size of all attachments
func (c *Store) Size() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.size
}

// Remaining returns the remaining capacity for attachments
func (c *Store) Remaining() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	remaining := c.limit - c.size
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Close stops the background sync goroutine and waits for it to finish
func (c *Store) Close() {
	close(c.closeChan)
	<-c.doneChan
}

func (c *Store) syncLoop() {
	defer close(c.doneChan)
	if err := c.sync(); err != nil {
		log.Tag(tagStore).Err(err).Warn("Attachment sync failed")
	}
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := c.sync(); err != nil {
				log.Tag(tagStore).Err(err).Warn("Attachment sync failed")
			}
		case <-c.closeChan:
			return
		}
	}
}
