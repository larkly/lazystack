package image

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// errUploadReplayed is returned when the HTTP layer tries to resend an
// upload body without rewinding it first. Failing loudly here prevents an
// empty or partial second attempt from being accepted as a complete image.
var errUploadReplayed = errors.New("upload body replayed without rewind")

// UploadReader feeds exactly Size bytes of image data to an upload request.
//
// It is seekable so the SDK can rewind and resend the full body when a
// request is retried after reauthentication, and it fails the request when
// the source turns out shorter or longer than the size it was created with
// (for example a local file that changed after it was inspected). BytesRead
// may be called concurrently with Read to drive progress displays.
type UploadReader struct {
	mu   sync.Mutex
	src  io.ReadSeeker
	size int64
	done bool // io.EOF was returned; further reads need a rewind first

	read atomic.Int64
}

// NewUploadReader wraps src, positioned at its start, which must yield
// exactly size bytes.
func NewUploadReader(src io.ReadSeeker, size int64) *UploadReader {
	return &UploadReader{src: src, size: size}
}

// Size returns the number of bytes the upload must contain.
func (u *UploadReader) Size() int64 { return u.size }

// BytesRead returns the bytes delivered in the current attempt.
func (u *UploadReader) BytesRead() int64 { return u.read.Load() }

// Read implements io.Reader.
func (u *UploadReader) Read(p []byte) (int, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.done {
		return 0, errUploadReplayed
	}
	read := u.read.Load()
	remaining := u.size - read
	if remaining <= 0 {
		// All expected bytes were delivered; the source must be exhausted.
		var extra [1]byte
		n, err := io.ReadFull(u.src, extra[:])
		if n > 0 {
			return 0, fmt.Errorf("image data is larger than the expected %d bytes (file changed during upload?)", u.size)
		}
		if err != io.EOF {
			return 0, err
		}
		u.done = true
		return 0, io.EOF
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := u.src.Read(p)
	u.read.Add(int64(n))
	if err == io.EOF {
		if read+int64(n) < u.size {
			return n, fmt.Errorf("image data ended after %d of %d bytes (file changed during upload?): %w",
				read+int64(n), u.size, io.ErrUnexpectedEOF)
		}
		// Report EOF on the next call, after verifying the source is exhausted.
		err = nil
	}
	return n, err
}

// Seek implements io.Seeker so a retried request can resend the full body.
func (u *UploadReader) Seek(offset int64, whence int) (int64, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	pos, err := u.src.Seek(offset, whence)
	if err != nil {
		return pos, err
	}
	u.read.Store(pos)
	u.done = false
	return pos, nil
}

// ParseMinimum parses an image min_disk (GB) or min_ram (MB) form value.
// Blank means the Glance default of 0; anything else must be a non-negative
// integer that fits Glance's 32-bit column.
func ParseMinimum(field, s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || n > math.MaxInt32 {
		return 0, fmt.Errorf("%s must be a whole number between 0 and %d", field, math.MaxInt32)
	}
	return int(n), nil
}
