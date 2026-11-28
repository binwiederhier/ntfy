package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
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
	require.Equal(t, "https", cfg.Scheme)
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
	require.Equal(t, "https", cfg.Scheme)
	require.Equal(t, "s3.example.com", cfg.Endpoint)
	require.True(t, cfg.PathStyle)
}

func TestParseURL_EndpointHTTP(t *testing.T) {
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket?region=us-east-1&endpoint=http://localhost:9000")
	require.Nil(t, err)
	require.Equal(t, "http", cfg.Scheme)
	require.Equal(t, "localhost:9000", cfg.Endpoint)
	require.True(t, cfg.PathStyle)
}

func TestParseURL_EndpointNoScheme(t *testing.T) {
	// A bare host:port endpoint (no scheme) must default to https for backward compatibility.
	// Without this, url.Parse treats the host as the scheme ("localhost:9000" -> scheme "localhost").
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket?region=us-east-1&endpoint=localhost:9000")
	require.Nil(t, err)
	require.Equal(t, "https", cfg.Scheme)
	require.Equal(t, "localhost:9000", cfg.Endpoint)
	require.True(t, cfg.PathStyle)
	require.Equal(t, "https://localhost:9000/my-bucket", cfg.BucketURL())
}

func TestParseURL_EndpointNoSchemeHostname(t *testing.T) {
	// A dotted hostname with a port and no scheme must also default to https
	// ("minio.example.com:9000" must not become scheme "minio.example.com").
	cfg, err := ParseURL("s3://AKID:SECRET@my-bucket?region=us-east-1&endpoint=minio.example.com:9000")
	require.Nil(t, err)
	require.Equal(t, "https", cfg.Scheme)
	require.Equal(t, "minio.example.com:9000", cfg.Endpoint)
	require.Equal(t, "https://minio.example.com:9000/my-bucket", cfg.BucketURL())
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

func TestConfig_BucketURL_PathStyle_EndpointHTTP(t *testing.T) {
	c := &Config{Scheme: "http", Endpoint: "localhost:9000", Bucket: "b", PathStyle: true}
	require.Equal(t, "http://localhost:9000/b", c.BucketURL())
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

// --- Tests using a fake in-process S3 server ---

func TestClient_PutObject_Fake_Small(t *testing.T) {
	fake, client := newFakeS3(t)
	data := []byte("hello world")
	require.Nil(t, client.PutObject(context.Background(), "small", bytes.NewReader(data), 0))
	require.Equal(t, data, fake.object("small"))
	require.Equal(t, 0, fake.multipartUploads())
}

func TestClient_PutObject_Fake_Empty(t *testing.T) {
	fake, client := newFakeS3(t)
	require.Nil(t, client.PutObject(context.Background(), "empty", bytes.NewReader(nil), 0))
	require.Equal(t, []byte{}, fake.object("empty"))
}

func TestClient_PutObject_Fake_ExactPartSize(t *testing.T) {
	fake, client := newFakeS3(t)
	data := testPattern(partSize)
	require.Nil(t, client.PutObject(context.Background(), "exact", bytes.NewReader(data), 0))
	require.Equal(t, data, fake.object("exact"))
	require.Equal(t, []int{partSize}, fake.partSizes())
}

func TestClient_PutObject_Fake_Multipart(t *testing.T) {
	fake, client := newFakeS3(t)
	data := testPattern(2*partSize + 12345)
	reader := &trickleReader{data: data, chunk: 100 * 1024}
	require.Nil(t, client.PutObject(context.Background(), "multi", reader, 0))
	require.Equal(t, data, fake.object("multi"))
	require.Equal(t, []int{partSize, partSize, 12345}, fake.partSizes())
}

func TestClient_PutObject_Fake_ReadErrorAbortsMultipart(t *testing.T) {
	fake, client := newFakeS3(t)
	readErr := errors.New("connection reset")
	body := io.MultiReader(bytes.NewReader(testPattern(partSize+100)), &errReader{err: readErr})
	err := client.PutObject(context.Background(), "broken", body, 0)
	require.ErrorIs(t, err, readErr)
	require.Nil(t, fake.object("broken"))
	require.Equal(t, 1, fake.aborts())
	require.Equal(t, []int{partSize}, fake.partSizes())
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

func TestClient_PutObject_BufferedSizes(t *testing.T) {
	// Bodies of unknown size are buffered in partChunkSize chunks and uploaded with a simple PUT up
	// to partSize, and as a multipart upload beyond that. These sizes sit on either side of the chunk
	// and part boundaries, so every way of assembling a request body from chunks is used.
	client := newTestClient(t)
	ctx := context.Background()
	sizes := []int{
		0,
		1,
		partChunkSize - 1,
		partChunkSize,
		partChunkSize + 1,
		1024 * 1024,                    // Several full chunks
		partSize - 1,                   // Largest simple PUT that ends inside a chunk
		partSize,                       // Largest simple PUT
		partSize + 1,                   // Smallest multipart upload
		2 * partSize,                   // Multipart, body ends exactly at a part boundary (no empty last part)
		2*partSize + partChunkSize + 7, // Multipart, last part ends inside a chunk
		12 * 1024 * 1024,               // Multipart, last part ends at a chunk boundary
	}
	for _, size := range sizes {
		key := fmt.Sprintf("size-%d", size)
		data := testPattern(size)
		require.Nil(t, client.PutObject(ctx, key, bytes.NewReader(data), 0), key)
		reader, gotSize, err := client.GetObject(ctx, key)
		require.Nil(t, err, key)
		require.Equal(t, int64(size), gotSize, key)
		got, err := io.ReadAll(reader)
		reader.Close()
		require.Nil(t, err, key)
		require.Equal(t, data, got, key)
	}
}

func TestClient_PutObject_SlowMultipart(t *testing.T) {
	// A slow client sends small reads; parts must still be filled completely before uploading
	client := newTestClient(t)
	ctx := context.Background()
	data := testPattern(partSize + 3*partChunkSize + 5)
	require.Nil(t, client.PutObject(ctx, "slow", &trickleReader{data: data, chunk: 1000}, 0))
	reader, size, err := client.GetObject(ctx, "slow")
	require.Nil(t, err)
	require.Equal(t, int64(len(data)), size)
	got, err := io.ReadAll(reader)
	reader.Close()
	require.Nil(t, err)
	require.Equal(t, data, got)
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

// trickleReader returns data in small chunks, like a slow network client
type trickleReader struct {
	data  []byte
	chunk int
}

func (r *trickleReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.chunk, len(r.data))
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

// errReader always fails with err
type errReader struct {
	err error
}

func (r *errReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

// fakeS3 is a minimal in-process S3 server that implements simple PUT and multipart uploads
type fakeS3 struct {
	objects map[string][]byte
	parts   map[int][]byte // part number -> data, of the current multipart upload
	uploads int
	aborted int
	mu      sync.Mutex // Protects all fields above
}

func newFakeS3(t *testing.T) (*fakeS3, *Client) {
	fake := &fakeS3{
		objects: make(map[string][]byte),
		parts:   make(map[int][]byte),
	}
	server := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(server.Close)
	client := New(&Config{
		Scheme:    "http",
		Endpoint:  strings.TrimPrefix(server.URL, "http://"),
		PathStyle: true,
		Bucket:    "bucket",
		Region:    "us-east-1",
		AccessKey: "AKID",
		SecretKey: "SECRET",
	})
	return fake, client
}

func (f *fakeS3) handle(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	query := r.URL.Query()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if r.ContentLength != int64(len(body)) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && query.Has("uploads"):
		f.uploads++
		f.parts = make(map[int][]byte)
		fmt.Fprint(w, `<InitiateMultipartUploadResult><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`)
	case r.Method == http.MethodPut && query.Has("partNumber"):
		partNumber, _ := strconv.Atoi(query.Get("partNumber"))
		f.parts[partNumber] = body
		w.Header().Set("ETag", fmt.Sprintf(`"etag-%d"`, partNumber))
	case r.Method == http.MethodPost && query.Has("uploadId"):
		var object []byte
		for _, n := range f.partNumbersNoLock() {
			object = append(object, f.parts[n]...)
		}
		f.objects[key] = object
		fmt.Fprint(w, `<CompleteMultipartUploadResult></CompleteMultipartUploadResult>`)
	case r.Method == http.MethodDelete && query.Has("uploadId"):
		f.aborted++
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut:
		f.objects[key] = body
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func (f *fakeS3) object(key string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.objects[key]
}

func (f *fakeS3) multipartUploads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uploads
}

func (f *fakeS3) aborts() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.aborted
}

func (f *fakeS3) partSizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	var sizes []int
	for _, n := range f.partNumbersNoLock() {
		sizes = append(sizes, len(f.parts[n]))
	}
	return sizes
}

func (f *fakeS3) partNumbersNoLock() []int {
	numbers := make([]int, 0, len(f.parts))
	for n := range f.parts {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	return numbers
}

func testPattern(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	return data
}
