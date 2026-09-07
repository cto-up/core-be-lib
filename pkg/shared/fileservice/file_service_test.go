package service

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/memblob"
)

// Range support and streaming delivery for GetFile.
//
// A browser will not let a user scrub a <video>/<audio> element unless the
// origin answers a ranged request with 206 — a 200 yields media that plays from
// the start and cannot be sought, and nothing in the console says why. So these
// are not "nice to have" cases: 200-where-206-was-asked-for is a silent
// feature outage.

func TestResolveRange(t *testing.T) {
	const size = 1000

	cases := []struct {
		name       string
		header     string
		wantOffset int64
		wantLength int64
		wantStatus int
		wantErr    bool
	}{
		{"absent serves the whole object", "", 0, size, http.StatusOK, false},
		{"closed range", "bytes=0-99", 0, 100, http.StatusPartialContent, false},
		{"mid-file closed range", "bytes=1000-1099", 0, 0, 0, true}, // start == size
		{"closed range inside", "bytes=500-599", 500, 100, http.StatusPartialContent, false},
		{"open-ended", "bytes=500-", 500, 500, http.StatusPartialContent, false},
		{"open-ended from zero", "bytes=0-", 0, 1000, http.StatusPartialContent, false},
		{"suffix", "bytes=-1024", 0, 1000, http.StatusPartialContent, false},
		{"suffix smaller than file", "bytes=-100", 900, 100, http.StatusPartialContent, false},
		{"end past EOF is clamped", "bytes=900-99999", 900, 100, http.StatusPartialContent, false},
		{"last byte", "bytes=999-999", 999, 1, http.StatusPartialContent, false},
		{"whitespace tolerated", " bytes=0-9 ", 0, 10, http.StatusPartialContent, false},

		// Unsatisfiable: the client's idea of the size is wrong and it must be
		// told, or it will retry the same impossible request forever.
		{"start past EOF", "bytes=5000-", 0, 0, 0, true},
		{"reversed", "bytes=500-100", 0, 0, 0, true},
		{"zero-length suffix", "bytes=-0", 0, 0, 0, true},

		// Malformed headers are IGNORED, per RFC 9110 — the client gets the whole
		// body rather than an error it did not ask for.
		{"no unit", "0-99", 0, size, http.StatusOK, false},
		{"wrong unit", "items=0-99", 0, size, http.StatusOK, false},
		{"garbage", "bytes=abc-def", 0, size, http.StatusOK, false},
		{"no hyphen", "bytes=100", 0, size, http.StatusOK, false},
		{"empty spec", "bytes=-", 0, size, http.StatusOK, false},
		{"multi-range unsupported, serve whole", "bytes=0-9,20-29", 0, size, http.StatusOK, false},
		// Not a suffix range with junk after it — the whole spec is malformed,
		// so it is ignored rather than half-interpreted.
		{"suffix with trailing junk", "bytes=-100-200", 0, size, http.StatusOK, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			offset, length, status, err := resolveRange(tc.header, size)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveRange(%q) = (%d,%d,%d), want an error",
						tc.header, offset, length, status)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRange(%q) unexpected error: %v", tc.header, err)
			}
			if offset != tc.wantOffset || length != tc.wantLength || status != tc.wantStatus {
				t.Errorf("resolveRange(%q) = (%d,%d,%d), want (%d,%d,%d)",
					tc.header, offset, length, status,
					tc.wantOffset, tc.wantLength, tc.wantStatus)
			}
		})
	}
}

func TestResolveRange_NeverExceedsTheObject(t *testing.T) {
	// Whatever a client asks for, the slice we hand to NewRangeReader must lie
	// inside the object — an out-of-bounds read is a 500 at best.
	const size = 64
	headers := []string{
		"", "bytes=0-", "bytes=-1", "bytes=-9999", "bytes=0-9999",
		"bytes=63-", "bytes=63-63", "bytes=0-0", "bytes=10-20",
	}
	for _, h := range headers {
		offset, length, _, err := resolveRange(h, size)
		if err != nil {
			continue
		}
		if offset < 0 || length < 0 || offset+length > size {
			t.Errorf("resolveRange(%q) = (%d,%d) escapes a %d-byte object", h, offset, length, size)
		}
	}
}

// newTestFileService returns a service backed by an in-memory bucket holding
// `content` at `name`.
func newTestFileService(t *testing.T, name string, content []byte) *FileService {
	t.Helper()
	bucket, err := blob.OpenBucket(context.Background(), "mem://")
	if err != nil {
		t.Fatalf("open mem bucket: %v", err)
	}
	t.Cleanup(func() { _ = bucket.Close() })

	if err := bucket.WriteAll(context.Background(), name, content, nil); err != nil {
		t.Fatalf("seed bucket: %v", err)
	}
	return &FileService{bucket: bucket}
}

func serve(t *testing.T, fs *FileService, name string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/"+name, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	ctx.Request = req
	_ = fs.GetFile(ctx, name)
	// gin's engine flushes a deferred status after the handler chain returns;
	// calling a handler directly skips that, and a response that sets a status
	// without writing a body (304) would otherwise still read as 200 here.
	ctx.Writer.WriteHeaderNow()
	return rec
}

func TestGetFile_WholeObject(t *testing.T) {
	body := []byte(strings.Repeat("abcdefghij", 100)) // 1000 bytes
	fs := newTestFileService(t, "video.mp4", body)

	rec := serve(t, fs, "video.mp4", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q, want \"bytes\" — without it no client will even try to seek", got)
	}
	if rec.Body.Len() != len(body) {
		t.Errorf("body = %d bytes, want %d", rec.Body.Len(), len(body))
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", got)
	}
}

func TestGetFile_RangeReturns206(t *testing.T) {
	body := []byte(strings.Repeat("abcdefghij", 100))
	fs := newTestFileService(t, "video.mp4", body)

	rec := serve(t, fs, "video.mp4", map[string]string{"Range": "bytes=100-199"})

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206 — a 200 here is a video that plays but cannot seek", rec.Code)
	}
	if got, want := rec.Header().Get("Content-Range"), "bytes 100-199/1000"; got != want {
		t.Errorf("Content-Range = %q, want %q", got, want)
	}
	if got := rec.Body.Len(); got != 100 {
		t.Fatalf("body = %d bytes, want 100", got)
	}
	if got, want := rec.Body.String(), string(body[100:200]); got != want {
		t.Errorf("body = %q, want %q — the wrong slice is worse than no slice", got, want)
	}
	if got := rec.Header().Get("Content-Length"); got != "100" {
		t.Errorf("Content-Length = %q, want 100", got)
	}
}

func TestGetFile_SeekBackAndForth(t *testing.T) {
	// AS-03: seek to 80% then back to 20%. Each request must return its own
	// slice — the failure this catches is a handler that ignores the offset and
	// returns the head of the file every time.
	body := make([]byte, 1000)
	for i := range body {
		body[i] = byte(i % 251)
	}
	fs := newTestFileService(t, "clip.mp4", body)

	for _, at := range []int64{800, 200, 999, 0} {
		rec := serve(t, fs, "clip.mp4", map[string]string{
			"Range": fmt.Sprintf("bytes=%d-%d", at, at+9),
		})
		if rec.Code != http.StatusPartialContent {
			t.Fatalf("seek to %d: status = %d, want 206", at, rec.Code)
		}
		end := at + 10
		if end > int64(len(body)) {
			end = int64(len(body))
		}
		if got, want := rec.Body.String(), string(body[at:end]); got != want {
			t.Errorf("seek to %d returned the wrong bytes", at)
		}
	}
}

func TestGetFile_SuffixRange(t *testing.T) {
	// How a PDF reader finds the trailer without fetching the whole document.
	body := []byte(strings.Repeat("x", 900) + "TRAILER!!")
	fs := newTestFileService(t, "doc.pdf", body)

	rec := serve(t, fs, "doc.pdf", map[string]string{"Range": "bytes=-9"})

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", rec.Code)
	}
	if got, want := rec.Body.String(), "TRAILER!!"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Content-Range"), "bytes 900-908/909"; got != want {
		t.Errorf("Content-Range = %q, want %q", got, want)
	}
}

func TestGetFile_OpenEndedRange(t *testing.T) {
	body := []byte(strings.Repeat("z", 500))
	fs := newTestFileService(t, "a.bin", body)

	rec := serve(t, fs, "a.bin", map[string]string{"Range": "bytes=400-"})

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", rec.Code)
	}
	if got, want := rec.Header().Get("Content-Range"), "bytes 400-499/500"; got != want {
		t.Errorf("Content-Range = %q, want %q", got, want)
	}
	if rec.Body.Len() != 100 {
		t.Errorf("body = %d bytes, want 100", rec.Body.Len())
	}
}

func TestGetFile_UnsatisfiableRange(t *testing.T) {
	fs := newTestFileService(t, "a.bin", []byte("small"))

	rec := serve(t, fs, "a.bin", map[string]string{"Range": "bytes=9999-"})

	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want 416", rec.Code)
	}
	if got, want := rec.Header().Get("Content-Range"), "bytes */5"; got != want {
		// The real size is what lets the client correct itself instead of
		// retrying the same impossible request.
		t.Errorf("Content-Range = %q, want %q", got, want)
	}
}

func TestGetFile_ETagRevalidation(t *testing.T) {
	body := []byte("hello world")
	fs := newTestFileService(t, "a.txt", body)

	first := serve(t, fs, "a.txt", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the first response")
	}

	second := serve(t, fs, "a.txt", map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 carried %d bytes of body", second.Body.Len())
	}
}

func TestGetFile_ETagMatchesContentHashWhenTheDriverReportsOne(t *testing.T) {
	// Backward compatibility: the previous implementation hashed the bytes
	// itself. Where the driver reports a content MD5 (S3, GCS, memblob) the new
	// metadata-derived validator reproduces the same value, so caches that were
	// populated before this change stay valid.
	body := []byte("hello world")
	fs := newTestFileService(t, "a.txt", body)

	rec := serve(t, fs, "a.txt", nil)

	want := fmt.Sprintf(`"%x"`, md5.Sum(body))
	if got := rec.Header().Get("ETag"); got != want {
		t.Errorf("ETag = %s, want %s (existing caches would be invalidated)", got, want)
	}
}

func TestGetFile_ETagDiffersAfterContentChanges(t *testing.T) {
	fs := newTestFileService(t, "a.txt", []byte("before"))
	before := serve(t, fs, "a.txt", nil).Header().Get("ETag")

	if err := fs.bucket.WriteAll(context.Background(), "a.txt", []byte("after!"), nil); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	after := serve(t, fs, "a.txt", nil).Header().Get("ETag")

	// Same length on purpose: a validator that only tracked size would miss it.
	if before == after {
		t.Error("ETag unchanged after the content changed — clients would serve stale bytes")
	}
}

func TestGetFile_Missing(t *testing.T) {
	fs := newTestFileService(t, "a.txt", []byte("x"))

	rec := serve(t, fs, "nope.txt", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestGetFile_RangeOnAMissingObjectIs404NotEmpty206(t *testing.T) {
	fs := newTestFileService(t, "a.txt", []byte("x"))

	rec := serve(t, fs, "nope.txt", map[string]string{"Range": "bytes=0-9"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
