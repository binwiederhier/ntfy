package attachment

import (
	"context"
	"io"
	"time"

	"heckel.io/ntfy/v2/s3"
)

type s3Backend struct {
	client *s3.Client
}

var _ backend = (*s3Backend)(nil)

func newS3Backend(client *s3.Client) *s3Backend {
	return &s3Backend{client: client}
}

func (b *s3Backend) Put(id string, reader io.Reader, untrustedLength int64) error {
	return b.client.PutObject(context.Background(), id, reader, untrustedLength)
}

func (b *s3Backend) Get(id string) (io.ReadCloser, int64, error) {
	return b.client.GetObject(context.Background(), id)
}

func (b *s3Backend) List() ([]object, error) {
	objects, err := b.client.ListObjectsV2(context.Background())
	if err != nil {
		return nil, err
	}
	result := make([]object, 0, len(objects))
	for _, obj := range objects {
		result = append(result, object{
			ID:           obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
		})
	}
	return result, nil
}

func (b *s3Backend) Delete(ids ...string) error {
	return b.client.DeleteObjects(context.Background(), ids)
}

func (b *s3Backend) DeleteIncomplete(cutoff time.Time) error {
	return b.client.AbortIncompleteUploads(context.Background(), cutoff)
}
