package service

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"strings"
	"testing"

	"gocloud.dev/blob"
)

func streamingFS(t *testing.T) *FileService {
	t.Helper()
	bucket, err := blob.OpenBucket(context.Background(), "mem://")
	if err != nil {
		t.Fatalf("open mem bucket: %v", err)
	}
	t.Cleanup(func() { _ = bucket.Close() })
	return &FileService{bucket: bucket}
}

// TestSaveFileStreamRoundTrips is the basic contract: what goes in comes back.
func TestSaveFileStreamRoundTrips(t *testing.T) {
	fs := streamingFS(t)
	body := "name,city\nada,london\n"

	n, err := fs.SaveFileStream(context.Background(), strings.NewReader(body), "a/b.csv")
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(body)) {
		t.Errorf("wrote %d bytes, want %d", n, len(body))
	}

	rc, err := fs.OpenFile(context.Background(), "a/b.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Errorf("read back %q", string(got))
	}
}

// TestSaveFileStreamDoesNotBufferTheObject is hub#125's actual claim.
//
// SaveFile takes a []byte, so every caller had already materialised the object
// before the write even began. The signature is what makes streaming possible,
// so the test measures allocation rather than trusting the shape: a 64 MiB body
// must not cost 64 MiB of heap.
func TestSaveFileStreamDoesNotBufferTheObject(t *testing.T) {
	fs := streamingFS(t)
	const size = 64 << 20

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	// A reader that yields `size` bytes without ever holding them: any
	// buffering on the write path shows up as heap that this side did not
	// allocate.
	n, err := fs.SaveFileStream(context.Background(), io.LimitReader(zeroes{}, size), "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if n != size {
		t.Fatalf("wrote %d, want %d", n, size)
	}

	runtime.ReadMemStats(&after)
	// memblob keeps the object itself in memory, which is the point of the
	// fake — so the ceiling is "one copy, not two". A buffering implementation
	// allocates the whole body on the way in as well.
	grew := int64(after.TotalAlloc - before.TotalAlloc)
	if grew > 3*size {
		t.Errorf("streaming a %d MiB object allocated %d MiB; it is being buffered on the way in",
			size>>20, grew>>20)
	}
}

type zeroes struct{}

func (zeroes) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// TestSaveFileStreamLeavesNoHalfObject: a key holding half an object is worse
// than no key, because the row that points at it looks fine.
func TestSaveFileStreamLeavesNoHalfObject(t *testing.T) {
	fs := streamingFS(t)

	_, err := fs.SaveFileStream(context.Background(),
		io.MultiReader(bytes.NewReader([]byte("good start")), errorReader{}), "partial.bin")
	if err == nil {
		t.Fatal("want the write to fail")
	}
	if _, err := fs.OpenFile(context.Background(), "partial.bin"); err == nil {
		t.Error("a partial object was left behind")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
