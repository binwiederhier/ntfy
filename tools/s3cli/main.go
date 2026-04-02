// Command s3cli is a minimal CLI for testing the s3 package. It supports put, get, rm, and ls.
//
// Usage:
//
//	export S3_URL="s3://ACCESS_KEY:SECRET_KEY@BUCKET/PREFIX?region=REGION&endpoint=ENDPOINT"
//
//	s3cli put <key> <file>       Upload a file
//	s3cli put <key> -            Upload from stdin
//	s3cli get <key>              Download to stdout
//	s3cli rm  <key> [<key>...]   Delete one or more objects
//	s3cli ls                     List all objects
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"heckel.io/ntfy/v2/s3"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	s3URL := os.Getenv("S3_URL")
	if s3URL == "" {
		fail("S3_URL environment variable is required")
	}
	cfg, err := s3.ParseURL(s3URL)
	if err != nil {
		fail("invalid S3_URL: %s", err)
	}
	client := s3.New(cfg)
	ctx := context.Background()

	switch os.Args[1] {
	case "put":
		cmdPut(ctx, client)
	case "get":
		cmdGet(ctx, client)
	case "rm":
		cmdRm(ctx, client)
	case "ls":
		cmdLs(ctx, client)
	default:
		usage()
	}
}

func cmdPut(ctx context.Context, client *s3.Client) {
	if len(os.Args) != 4 {
		fail("usage: s3cli put <key> <file|->\n")
	}
	key := os.Args[2]
	path := os.Args[3]

	var r io.Reader
	var size int64
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			fail("open %s: %s", path, err)
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil {
			fail("stat %s: %s", path, err)
		}
		r = f
		size = stat.Size()
	}

	if err := client.PutObject(ctx, key, r, size); err != nil {
		fail("put: %s", err)
	}
	fmt.Fprintf(os.Stderr, "uploaded %s\n", key)
}

func cmdGet(ctx context.Context, client *s3.Client) {
	if len(os.Args) != 3 {
		fail("usage: s3cli get <key>\n")
	}
	key := os.Args[2]

	reader, size, err := client.GetObject(ctx, key)
	if err != nil {
		fail("get: %s", err)
	}
	defer reader.Close()
	n, err := io.Copy(os.Stdout, reader)
	if err != nil {
		fail("read: %s", err)
	}
	fmt.Fprintf(os.Stderr, "downloaded %s (%d bytes, content-length: %d)\n", key, n, size)
}

func cmdRm(ctx context.Context, client *s3.Client) {
	if len(os.Args) < 3 {
		fail("usage: s3cli rm <key> [<key>...]\n")
	}
	keys := os.Args[2:]
	if err := client.DeleteObjects(ctx, keys); err != nil {
		fail("rm: %s", err)
	}
	fmt.Fprintf(os.Stderr, "deleted %d object(s)\n", len(keys))
}

func cmdLs(ctx context.Context, client *s3.Client) {
	objects, err := client.ListObjectsV2(ctx)
	if err != nil {
		fail("ls: %s", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	var totalSize int64
	for _, obj := range objects {
		fmt.Fprintf(w, "%d\t%s\n", obj.Size, obj.Key)
		totalSize += obj.Size
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "%d object(s), %d bytes total\n", len(objects), totalSize)
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: s3cli <command> [args...]

Commands:
  put <key> <file|->   Upload a file (use - for stdin)
  get <key>            Download to stdout
  rm  <key> [keys...]  Delete objects
  ls                   List all objects

Environment:
  S3_URL  S3 connection URL (required)
          s3://ACCESS_KEY:SECRET_KEY@BUCKET[/PREFIX]?region=REGION[&endpoint=ENDPOINT]
`)
	os.Exit(1)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
