package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseURL_Success(t *testing.T) {
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket/attachments?region=us-east-1")
	require.Nil(t, err)
	require.Equal(t, "my-bucket", cfg.Bucket)
	require.Equal(t, "attachments", cfg.Prefix)
	require.Equal(t, "us-east-1", cfg.Region)
	require.Equal(t, "AKID", cfg.AccessKey)
	require.Equal(t, "SECRET", cfg.SecretKey)
	require.Equal(t, "s3.us-east-1.amazonaws.com", cfg.Endpoint)
	require.False(t, cfg.PathStyle)
}

func TestParseURL_NoPrefix(t *testing.T) {
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket?region=us-east-1")
	require.Nil(t, err)
	require.Equal(t, "my-bucket", cfg.Bucket)
	require.Equal(t, "", cfg.Prefix)
}

func TestParseURL_WithEndpoint(t *testing.T) {
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket/prefix?region=us-east-1&endpoint=https://s3.example.com")
	require.Nil(t, err)
	require.Equal(t, "my-bucket", cfg.Bucket)
	require.Equal(t, "prefix", cfg.Prefix)
	require.Equal(t, "s3.example.com", cfg.Endpoint)
	require.True(t, cfg.PathStyle)
}

func TestParseURL_EndpointHTTP(t *testing.T) {
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket?region=us-east-1&endpoint=http://localhost:9000")
	require.Nil(t, err)
	require.Equal(t, "localhost:9000", cfg.Endpoint)
	require.True(t, cfg.PathStyle)
}

func TestParseURL_EndpointTrailingSlash(t *testing.T) {
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket?region=us-east-1&endpoint=https://s3.example.com/")
	require.Nil(t, err)
	require.Equal(t, "s3.example.com", cfg.Endpoint)
}

func TestParseURL_NestedPrefix(t *testing.T) {
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket/a/b/c?region=us-east-1")
	require.Nil(t, err)
	require.Equal(t, "my-bucket", cfg.Bucket)
	require.Equal(t, "a/b/c", cfg.Prefix)
}

func TestParseURL_MissingRegion(t *testing.T) {
	_, err := ParseURL("s3://AKID:SECRET@my-bucket")
	require.Error(t, err)
	require.Contains(t, err.Error(), "region")
}

func TestParseURL_MissingCredentials(t *testing.T) {
	_, err := ParseURL("s3://my-bucket?region=us-east-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "access key")
}

func TestParseURL_MissingSecretKey(t *testing.T) {
	_, err := ParseURL("s3://AKID@my-bucket?region=us-east-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "secret key")
}

func TestParseURL_WrongScheme(t *testing.T) {
	_, err := ParseURL("http://AKID:SECRET@my-bucket?region=us-east-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "scheme")
}

func TestParseURL_EmptyBucket(t *testing.T) {
	_, err := ParseURL("s3://AKID:SECRET@?region=us-east-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "bucket")
}

func TestParseURL_DisableHTTP2(t *testing.T) {
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket?region=us-east-1&disable_http2=true")
	require.Nil(t, err)
	require.True(t, cfg.DisableHTTP2)
}

func TestParseURL_DisableHTTP2_NotSet(t *testing.T) {
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket?region=us-east-1")
	require.Nil(t, err)
	require.False(t, cfg.DisableHTTP2)
}

// --- Unit tests: URL construction ---

func TestConfig_BucketURL_PathStyle(t *testing.T) {
	c := &Config{Endpoint: "s3.example.com", Bucket: "my-bucket", PathStyle: true}
	require.Equal(t, "https://s3.example.com/my-bucket", c.BucketURL())
}

func TestConfig_BucketURL_VirtualHosted(t *testing.T) {
	c := &Config{Endpoint: "s3.us-east-1.amazonaws.com", Bucket: "my-bucket", PathStyle: false}
	require.Equal(t, "https://my-bucket.s3.us-east-1.amazonaws.com", c.BucketURL())
}

func TestConfig_ObjectURL_PathStyle(t *testing.T) {
	c := &Config{Endpoint: "s3.example.com", Bucket: "my-bucket", Prefix: "prefix", PathStyle: true}
	require.Equal(t, "https://s3.example.com/my-bucket/prefix/obj", c.ObjectURL("obj"))
}

func TestConfig_ObjectURL_VirtualHosted(t *testing.T) {
	c := &Config{Endpoint: "s3.us-east-1.amazonaws.com", Bucket: "my-bucket", Prefix: "prefix", PathStyle: false}
	require.Equal(t, "https://my-bucket.s3.us-east-1.amazonaws.com/prefix/obj", c.ObjectURL("obj"))
}

func TestConfig_HostHeader_PathStyle(t *testing.T) {
	c := &Config{Endpoint: "s3.example.com", Bucket: "my-bucket", PathStyle: true}
	require.Equal(t, "s3.example.com", c.HostHeader())
}

func TestConfig_HostHeader_VirtualHosted(t *testing.T) {
	c := &Config{Endpoint: "s3.us-east-1.amazonaws.com", Bucket: "my-bucket", PathStyle: false}
	require.Equal(t, "my-bucket.s3.us-east-1.amazonaws.com", c.HostHeader())
}

func TestConfig_ObjectKey(t *testing.T) {
	c := &Config{Prefix: "attachments"}
	require.Equal(t, "attachments/file123", c.ObjectKey("file123"))

	c2 := &Config{Prefix: ""}
	require.Equal(t, "file123", c2.ObjectKey("file123"))
}

func TestConfig_ListPrefix(t *testing.T) {
	c := &Config{Prefix: "attachments"}
	require.Equal(t, "attachments/", c.ListPrefix())

	c2 := &Config{Prefix: ""}
	require.Equal(t, "", c2.ListPrefix())
}

// --- Integration tests using real S3 ---

func TestClient_PutGetObject(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// Put
	err := client.PutObject(ctx, "test-key", strings.NewReader("hello world"), 0)
	require.Nil(t, err)

	// Get
	reader, size, err := client.GetObject(ctx, "test-key")
	require.Nil(t, err)
	require.Equal(t, int64(11), size)
	data, err := io.ReadAll(reader)
	reader.Close()
	require.Nil(t, err)
	require.Equal(t, "hello world", string(data))
}

func TestClient_GetObject_NotFound(t *testing.T) {
	client := newTestClient(t)

	_, _, err := client.GetObject(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestClient_DeleteObjects(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// Put several objects
	for i := 0; i < 5; i++ {
		err := client.PutObject(ctx, fmt.Sprintf("del-%d", i), bytes.NewReader([]byte("data")), 0)
		require.Nil(t, err)
	}
	waitForCount(t, client, 5)

	// Delete some
	err := client.DeleteObjects(ctx, []string{"del-1", "del-3"})
	require.Nil(t, err)
	waitForCount(t, client, 3)

	// Verify deleted ones are gone
	_, _, err = client.GetObject(ctx, "del-1")
	require.Error(t, err)
	_, _, err = client.GetObject(ctx, "del-3")
	require.Error(t, err)

	// Verify remaining ones are still there
	for _, key := range []string{"del-0", "del-2", "del-4"} {
		reader, _, err := client.GetObject(ctx, key)
		require.Nil(t, err)
		reader.Close()
	}
}

func TestClient_ListObjects(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		err := client.PutObject(ctx, fmt.Sprintf("list-%d", i), bytes.NewReader([]byte("x")), 0)
		require.Nil(t, err)
	}
	waitForCount(t, client, 3)
}

func TestClient_ListObjects_Pagination(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// Create 1010 objects in parallel (5 goroutines)
	const total = 1010
	const workers = 5
	var wg sync.WaitGroup
	errs := make(chan error, total)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			for i := start; i < total; i += workers {
				if err := client.PutObject(ctx, fmt.Sprintf("pg-%04d", i), bytes.NewReader([]byte("x")), 0); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.Nil(t, err)
	}
	waitForCount(t, client, total)
}

func TestClient_PutObject_LargeBody(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// 1 MB object
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	err := client.PutObject(ctx, "large", bytes.NewReader(data), 0)
	require.Nil(t, err)

	reader, size, err := client.GetObject(ctx, "large")
	require.Nil(t, err)
	require.Equal(t, int64(1024*1024), size)
	got, err := io.ReadAll(reader)
	reader.Close()
	require.Nil(t, err)
	require.Equal(t, data, got)
}

func TestClient_PutObject_ChunkedUpload(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// 12 MB object, exceeds 5 MB partSize, triggers multipart upload path
	data := make([]byte, 12*1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	err := client.PutObject(ctx, "multipart", bytes.NewReader(data), 0)
	require.Nil(t, err)

	reader, size, err := client.GetObject(ctx, "multipart")
	require.Nil(t, err)
	require.Equal(t, int64(12*1024*1024), size)
	got, err := io.ReadAll(reader)
	reader.Close()
	require.Nil(t, err)
	require.Equal(t, data, got)
}

func TestClient_PutObject_ExactPartSize(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// Exactly 5 MB (partSize), should use the simple put path (ReadFull succeeds fully)
	data := make([]byte, 5*1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	err := client.PutObject(ctx, "exact", bytes.NewReader(data), 0)
	require.Nil(t, err)

	reader, size, err := client.GetObject(ctx, "exact")
	require.Nil(t, err)
	require.Equal(t, int64(5*1024*1024), size)
	got, err := io.ReadAll(reader)
	reader.Close()
	require.Nil(t, err)
	require.Equal(t, data, got)
}

func TestClient_PutObject_StreamingExactLength(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// untrustedLength matches body exactly — streams directly via putObject
	err := client.PutObject(ctx, "stream-exact", strings.NewReader("hello world"), 11)
	require.Nil(t, err)

	reader, size, err := client.GetObject(ctx, "stream-exact")
	require.Nil(t, err)
	require.Equal(t, int64(11), size)
	got, err := io.ReadAll(reader)
	reader.Close()
	require.Nil(t, err)
	require.Equal(t, "hello world", string(got))
}

func TestClient_PutObject_StreamingBodyLongerThanClaimed(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// Body has 11 bytes, but we claim 5 — only first 5 bytes should be stored
	err := client.PutObject(ctx, "stream-long", strings.NewReader("hello world"), 5)
	require.Nil(t, err)

	reader, size, err := client.GetObject(ctx, "stream-long")
	require.Nil(t, err)
	require.Equal(t, int64(5), size)
	got, err := io.ReadAll(reader)
	reader.Close()
	require.Nil(t, err)
	require.Equal(t, "hello", string(got))
}

func TestClient_PutObject_StreamingBodyShorterThanClaimed(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	// Body has 5 bytes, but we claim 100 — should fail
	err := client.PutObject(ctx, "stream-short", strings.NewReader("hello"), 100)
	require.Error(t, err)

	// Object should not exist
	_, _, err = client.GetObject(ctx, "stream-short")
	require.Error(t, err)
}

func TestClient_PutObject_NestedKey(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	err := client.PutObject(ctx, "deep/nested/prefix/file.txt", strings.NewReader("nested"), 0)
	require.Nil(t, err)

	reader, _, err := client.GetObject(ctx, "deep/nested/prefix/file.txt")
	require.Nil(t, err)
	data, _ := io.ReadAll(reader)
	reader.Close()
	require.Equal(t, "nested", string(data))
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	s3URL := os.Getenv("NTFY_TEST_S3_URL")
	if s3URL == "" {
		t.Skip("NTFY_TEST_S3_URL not set")
	}
	cfg, err := ParseURL(s3URL)
	require.Nil(t, err)
	// Use per-test prefix to isolate objects between tests
	if cfg.Prefix != "" {
		cfg.Prefix = cfg.Prefix + "/testpkg-s3/" + t.Name()
	} else {
		cfg.Prefix = "testpkg-s3/" + t.Name()
	}
	client := New(cfg)
	deleteAllObjects(t, client)
	t.Cleanup(func() { deleteAllObjects(t, client) })
	return client
}

func deleteAllObjects(t *testing.T, client *Client) {
	t.Helper()
	for i := 0; i < 60; i++ {
		objects, err := client.ListObjectsV2(context.Background())
		require.Nil(t, err)
		if len(objects) == 0 {
			return
		}
		keys := make([]string, len(objects))
		for j, obj := range objects {
			keys[j] = obj.Key
		}
		require.Nil(t, client.DeleteObjects(context.Background(), keys))
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("timed out waiting for bucket to be empty")
}

func waitForCount(t *testing.T, client *Client, expected int) {
	t.Helper()
	for i := 0; i < 60; i++ {
		objects, err := client.ListObjectsV2(context.Background())
		require.Nil(t, err)
		if len(objects) == expected {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	objects, _ := client.ListObjectsV2(context.Background())
	t.Fatalf("timed out waiting for %d objects, got %d", expected, len(objects))
}
