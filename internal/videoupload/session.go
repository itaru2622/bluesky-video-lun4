package videoupload

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
)

// Session states. These map 1:1 onto the "state" enum shared by
// app.bsky.video.getUploadStatus and app.bsky.video.abortUpload.
const (
	StateCreated   = "created"
	StateFinishing = "finishing"
	StateCompleted = "completed"
	StateFailed    = "failed"
	StateAborted   = "aborted"
	StateExpired   = "expired"
)

// session is the internal, mutex-protected representation of one
// in-progress (or recently-terminated) multipart video upload. All
// access must go through Manager, which holds sessions in a sync.Map
// keyed by jobID.
type session struct {
	mu sync.Mutex

	jobID    string
	userDID  string
	mimeType string
	name     string
	// token is the caller's Authorization header value captured at
	// startUpload time. It is expected to be a PDS-scoped service-auth
	// token (aud=PDS, lxm=com.atproto.repo.uploadBlob) and is forwarded
	// unchanged to the existing upload pipeline once finishUpload
	// assembles the full video, exactly like the pre-existing
	// (single-shot) uploadVideo endpoint already does.
	token string

	sizeBytes     int64
	partSizeBytes int64
	partCount     int64

	// dir holds one file per received part, named "<partNumber>.part".
	dir string
	// receivedSize tracks which parts have arrived and how many bytes
	// each one actually contains.
	receivedSize map[int64]int64

	state          string
	completedJobID string
	jobStatus      *bsky.VideoDefs_JobStatus
	failureReason  string

	createdAt time.Time
	expiresAt time.Time
}

func newSession(jobID, userDID, token, mimeType, name string, sizeBytes, partSizeBytes int64, dir string, ttl time.Duration) *session {
	partCount := sizeBytes / partSizeBytes
	if sizeBytes%partSizeBytes != 0 {
		partCount++
	}
	if partCount == 0 {
		partCount = 1
	}
	now := time.Now()
	return &session{
		jobID:         jobID,
		userDID:       userDID,
		token:         token,
		mimeType:      mimeType,
		name:          name,
		sizeBytes:     sizeBytes,
		partSizeBytes: partSizeBytes,
		partCount:     partCount,
		dir:           dir,
		receivedSize:  make(map[int64]int64),
		state:         StateCreated,
		createdAt:     now,
		expiresAt:     now.Add(ttl),
	}
}

// expectedPartSize returns the exact number of bytes the given
// (1-indexed) part must contain: partSizeBytes for every part except
// the last, which holds whatever remainder is left over.
func (s *session) expectedPartSize(partNumber int64) (int64, error) {
	if partNumber < 1 || partNumber > s.partCount {
		return 0, ErrBadPartNumber
	}
	if partNumber < s.partCount {
		return s.partSizeBytes, nil
	}
	return s.sizeBytes - s.partSizeBytes*(s.partCount-1), nil
}

func (s *session) partPath(partNumber int64) string {
	return filepath.Join(s.dir, fmt.Sprintf("%d.part", partNumber))
}

// receivedPartNumbers returns the currently-received part numbers,
// sorted ascending, as required by getUploadStatus#receivedParts.
func (s *session) receivedPartNumbers() []int64 {
	out := make([]int64, 0, len(s.receivedSize))
	for n := range s.receivedSize {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *session) isComplete() bool {
	return int64(len(s.receivedSize)) == s.partCount
}

// removeFiles deletes every part file (and the assembly scratch file, if
// any) backing this session. Safe to call multiple times.
func (s *session) removeFiles() {
	os.RemoveAll(s.dir)
}
