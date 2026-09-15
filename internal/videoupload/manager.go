package videoupload

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
	gonanoid "github.com/matoous/go-nanoid"

	bskyshim "github.com/itaru2622/bluesky-video-lun4/internal/api/bsky"
)

const (
	jobIDAlphabet = "abcdefghijklmnopqrstuvwxyz1234567890"
	jobIDLength   = 24

	// DefaultPartSizeBytes / DefaultSessionTTL are used whenever Config
	// leaves the corresponding field at its zero value.
	DefaultPartSizeBytes = 5 * 1024 * 1024 // 5 MiB
	DefaultSessionTTL    = 30 * time.Minute

	// sessionReapAge bounds how long a terminal session's bookkeeping
	// (not its files -- those are removed as soon as it goes terminal)
	// is kept around in memory for late getUploadStatus/abortUpload
	// polls, before the cleanup loop drops it entirely.
	sessionReapAge = 24 * time.Hour
)

// Config controls session sizing/lifetime and where in-flight part data
// is buffered on disk. Zero values fall back to the defaults above.
type Config struct {
	// PartSizeBytes is handed back to clients from startUpload and is
	// the exact chunk size every part but the last must have.
	PartSizeBytes int64
	// SessionTTL is how long a session may sit in "created" (i.e. not
	// yet finished) before it is reaped as expired.
	SessionTTL time.Duration
	// TempDir is the parent directory under which one subdirectory per
	// in-flight session is created to hold its parts. Defaults to
	// os.TempDir().
	TempDir string
	// MaxSizeBytes optionally caps the declared sizeBytes accepted by
	// startUpload. Zero means unlimited (enforce elsewhere, e.g. via
	// getUploadLimits/allowedDIDs, if desired).
	MaxSizeBytes int64
}

// FinishFunc hands the fully-assembled video file off to the caller for
// processing once every part has been received. It mirrors what the
// pre-existing (single-shot) uploadVideo handler already does with a
// whole-body upload: mint/reuse a processing job id, kick off any
// long-running work (e.g. forwarding the blob to the user's PDS) without
// blocking, and return that job's id plus its current status so
// finishUpload/getUploadStatus can report it. token is the value
// captured from the Authorization header at startUpload time.
type FinishFunc func(userDID, token string, video *os.File, mimeType, name string) (completedJobID string, status *bsky.VideoDefs_JobStatus, err error)

// Manager owns every in-flight (and recently-terminated) multipart
// upload session. It is safe for concurrent use.
type Manager struct {
	cfg   Config
	onFin FinishFunc
	byJob sync.Map // jobID (string) -> *session
}

// NewManager creates a Manager and starts its background expiry/cleanup
// loop. onFinish must not be nil.
func NewManager(cfg Config, onFinish FinishFunc) *Manager {
	if cfg.PartSizeBytes <= 0 {
		cfg.PartSizeBytes = DefaultPartSizeBytes
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = DefaultSessionTTL
	}
	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}
	m := &Manager{cfg: cfg, onFin: onFinish}
	go m.cleanupLoop()
	return m
}

func newJobID() string {
	return gonanoid.MustGenerate(jobIDAlphabet, jobIDLength)
}

// Start implements app.bsky.video.startUpload.
// TODO: no quota/limit enforcement yet (DailyLimitExceeded,
// TooManyOpenUploads, UploadForbidden, VideoTooLarge, VideoTooLong,
// BadAspectRatio, UnsupportedContentType are never returned); getUploadLimits
// isn't consulted here either.
func (m *Manager) Start(userDID, token string, in bskyshim.VideoStartUpload_Input) (*bskyshim.VideoStartUpload_Output, error) {
	if len(in.MimeType) < 3 || len(in.MimeType) > 255 {
		return nil, ErrInvalidMimeType
	}
	if in.SizeBytes < 1 {
		return nil, ErrInvalidSize
	}
	if m.cfg.MaxSizeBytes > 0 && in.SizeBytes > m.cfg.MaxSizeBytes {
		return nil, ErrInvalidSize
	}

	jobID := newJobID()
	dir, err := os.MkdirTemp(m.cfg.TempDir, "vidupload_"+jobID+"_")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate storage for upload: %w", err)
	}

	name := ""
	if in.Name != nil {
		name = *in.Name
	}

	sess := newSession(jobID, userDID, token, in.MimeType, name, in.SizeBytes, m.cfg.PartSizeBytes, dir, m.cfg.SessionTTL)
	m.byJob.Store(jobID, sess)

	return &bskyshim.VideoStartUpload_Output{
		JobId:         jobID,
		PartSizeBytes: sess.partSizeBytes,
		PartCount:     sess.partCount,
		ExpiresAt:     sess.expiresAt,
	}, nil
}

func (m *Manager) get(jobID string) (*session, error) {
	if jobID == "" {
		return nil, ErrNotFound
	}
	v, ok := m.byJob.Load(jobID)
	if !ok {
		return nil, ErrNotFound
	}
	return v.(*session), nil
}

// errorForBlockedState maps a session state other than StateCreated to
// the sentinel error callers that require StateCreated (WritePart,
// Finish) should surface, matching the distinct error names the atproto
// lexicons document per state (UploadNotReady/UploadFailed/UploadAborted/
// UploadAlreadyCompleted/UploadExpired).
func errorForBlockedState(state string) error {
	switch state {
	case StateFinishing:
		return ErrUploadNotReady
	case StateCompleted:
		return ErrAlreadyCompleted
	case StateFailed:
		return ErrUploadFailed
	case StateAborted:
		return ErrUploadAborted
	case StateExpired:
		return ErrExpired
	default:
		return ErrUploadNotReady
	}
}

// WritePart implements app.bsky.video.uploadPart. body is read fully and
// must contain exactly the expected number of bytes for partNumber.
// contentLength, when known (>= 0), is checked up front; pass -1 if the
// caller (e.g. chunked transfer-encoding) doesn't know it in advance.
// Parts are idempotent: re-uploading the same partNumber simply
// overwrites the previously-stored bytes for that part.
func (m *Manager) WritePart(jobID string, partNumber, contentLength int64, body io.Reader) (*bskyshim.VideoUploadPart_Output, error) {
	sess, err := m.get(jobID)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.state != StateCreated {
		return nil, errorForBlockedState(sess.state)
	}
	if time.Now().After(sess.expiresAt) {
		sess.state = StateExpired
		go sess.removeFiles()
		return nil, ErrExpired
	}

	expected, err := sess.expectedPartSize(partNumber)
	if err != nil {
		return nil, err
	}
	if contentLength >= 0 && contentLength != expected {
		return nil, ErrSizeMismatch
	}

	path := sess.partPath(partNumber)
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open part storage: %w", err)
	}
	// Read one byte past what's expected so an oversized part is
	// detected (n != expected) instead of silently truncated.
	n, copyErr := io.Copy(f, io.LimitReader(body, expected+1))
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(path)
		return nil, fmt.Errorf("failed to write part: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(path)
		return nil, fmt.Errorf("failed to write part: %w", closeErr)
	}
	if n != expected {
		os.Remove(path)
		return nil, ErrSizeMismatch
	}

	sess.receivedSize[partNumber] = n
	return &bskyshim.VideoUploadPart_Output{PartNumber: partNumber, SizeBytes: n}, nil
}

// Finish implements app.bsky.video.finishUpload. It is idempotent: once a
// session has left the "created" state, subsequent calls return the
// already-known terminal outcome (or ErrUploadNotReady while a first
// call is still assembling/handing off).
func (m *Manager) Finish(jobID string) (*bskyshim.VideoFinishUpload_Output, error) {
	sess, err := m.get(jobID)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	switch sess.state {
	case StateFinishing:
		sess.mu.Unlock()
		return nil, ErrUploadNotReady
	case StateCreated:
		if !sess.isComplete() {
			sess.mu.Unlock()
			return nil, ErrIncomplete
		}
		sess.state = StateFinishing
	case StateCompleted:
		// Idempotent retry: hand back the previously-completed outcome.
		out := &bskyshim.VideoFinishUpload_Output{CompletedJobId: sess.completedJobID, JobStatus: sess.jobStatus}
		sess.mu.Unlock()
		return out, nil
	default:
		// Failed/aborted/expired sessions cannot be finished.
		err := errorForBlockedState(sess.state)
		sess.mu.Unlock()
		return nil, err
	}
	partCount := sess.partCount
	mimeType := sess.mimeType
	name := sess.name
	userDID := sess.userDID
	token := sess.token
	dir := sess.dir
	sess.mu.Unlock()

	assembled, err := assembleParts(dir, partCount)
	if err != nil {
		m.fail(sess, err)
		return nil, err
	}
	defer func() {
		assembled.Close()
		os.Remove(assembled.Name())
	}()

	completedJobID, status, err := m.onFin(userDID, token, assembled, mimeType, name)
	if err != nil {
		m.fail(sess, err)
		return nil, err
	}

	sess.mu.Lock()
	sess.state = StateCompleted
	sess.completedJobID = completedJobID
	sess.jobStatus = status
	sess.mu.Unlock()
	sess.removeFiles()

	return &bskyshim.VideoFinishUpload_Output{CompletedJobId: completedJobID, JobStatus: status}, nil
}

func (m *Manager) fail(sess *session, cause error) {
	sess.mu.Lock()
	sess.state = StateFailed
	sess.failureReason = cause.Error()
	sess.mu.Unlock()
	sess.removeFiles()
}

// Abort implements app.bsky.video.abortUpload. It can only cancel a
// session that is still "created"; a session that is mid-finish returns
// ErrUploadNotReady, and a session that already reached a terminal state
// is left unchanged and its existing outcome is returned as-is.
func (m *Manager) Abort(jobID string) (*bskyshim.VideoAbortUpload_Output, error) {
	sess, err := m.get(jobID)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	switch sess.state {
	case StateCreated:
		sess.state = StateAborted
		go sess.removeFiles()
		return &bskyshim.VideoAbortUpload_Output{State: StateAborted}, nil
	case StateFinishing:
		return nil, ErrUploadNotReady
	default:
		out := &bskyshim.VideoAbortUpload_Output{State: sess.state}
		if sess.completedJobID != "" {
			id := sess.completedJobID
			out.CompletedJobId = &id
		}
		if sess.failureReason != "" {
			r := sess.failureReason
			out.FailureReason = &r
		}
		return out, nil
	}
}

// Status implements app.bsky.video.getUploadStatus.
func (m *Manager) Status(jobID string) (*bskyshim.VideoGetUploadStatus_Output, error) {
	sess, err := m.get(jobID)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.state == StateCreated && time.Now().After(sess.expiresAt) {
		sess.state = StateExpired
		go sess.removeFiles()
	}

	out := &bskyshim.VideoGetUploadStatus_Output{
		JobId:         sess.jobID,
		State:         sess.state,
		PartSizeBytes: sess.partSizeBytes,
		PartCount:     sess.partCount,
		ReceivedParts: sess.receivedPartNumbers(),
		ExpiresAt:     sess.expiresAt,
	}
	if sess.state == StateCompleted {
		id := sess.completedJobID
		out.CompletedJobId = &id
		out.JobStatus = sess.jobStatus
	}
	if sess.state == StateFailed && sess.failureReason != "" {
		r := sess.failureReason
		out.FailureReason = &r
	}
	return out, nil
}

// cleanupLoop periodically expires stale "created" sessions (releasing
// their part files) and, separately, drops long-terminal sessions from
// memory so the sync.Map doesn't grow unbounded over server uptime.
func (m *Manager) cleanupLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		m.byJob.Range(func(k, v any) bool {
			sess := v.(*session)

			sess.mu.Lock()
			expiredNow := sess.state == StateCreated && now.After(sess.expiresAt)
			if expiredNow {
				sess.state = StateExpired
			}
			isTerminal := sess.state == StateCompleted || sess.state == StateFailed ||
				sess.state == StateAborted || sess.state == StateExpired
			isOld := now.Sub(sess.createdAt) > sessionReapAge
			sess.mu.Unlock()

			if expiredNow {
				sess.removeFiles()
			}
			if isTerminal && isOld {
				m.byJob.Delete(k)
			}
			return true
		})
	}
}

// assembleParts concatenates dir/1.part..dir/partCount.part (in part
// order) into one scratch file inside dir and returns it opened for
// reading, seeked back to the start.
func assembleParts(dir string, partCount int64) (*os.File, error) {
	out, err := os.CreateTemp(dir, "assembled_")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate assembly buffer: %w", err)
	}
	for i := int64(1); i <= partCount; i++ {
		partPath := filepath.Join(dir, fmt.Sprintf("%d.part", i))
		part, err := os.Open(partPath)
		if err != nil {
			out.Close()
			return nil, fmt.Errorf("missing part %d: %w", i, err)
		}
		_, err = io.Copy(out, part)
		part.Close()
		if err != nil {
			out.Close()
			return nil, fmt.Errorf("failed to assemble part %d: %w", i, err)
		}
	}
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		out.Close()
		return nil, fmt.Errorf("failed to rewind assembled upload: %w", err)
	}
	return out, nil
}
