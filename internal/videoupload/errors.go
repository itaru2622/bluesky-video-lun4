package videoupload

import "errors"

// Sentinel errors returned by Manager methods. The gin handlers in
// main's video_upload_handlers.go map these onto HTTP status codes and
// atproto XRPC error names (e.g. "UploadNotReady").
var (
	// ErrNotFound maps to the atproto "UploadNotFound" error: jobId does
	// not correspond to any known upload session (never existed, or was
	// already garbage collected long after reaching a terminal state).
	ErrNotFound = errors.New("upload session not found")

	// ErrExpired maps to the atproto "UploadExpired" error: a session
	// sitting in "created" has outlived its SessionTTL.
	ErrExpired = errors.New("upload session expired")

	// ErrUploadNotReady mirrors the atproto "UploadNotReady" error: the
	// session is currently finishing (finishUpload already in flight)
	// and uploadPart/abortUpload/finishUpload cannot be called
	// concurrently.
	ErrUploadNotReady = errors.New("UploadNotReady")

	// ErrUploadFailed maps to the atproto "UploadFailed" error: the
	// session is known to have failed.
	ErrUploadFailed = errors.New("UploadFailed")

	// ErrUploadAborted maps to the atproto "UploadAborted" error: the
	// session is known to have been aborted.
	ErrUploadAborted = errors.New("UploadAborted")

	// ErrAlreadyCompleted maps to the atproto "UploadAlreadyCompleted"
	// error, returned by WritePart when the session has already
	// completed (uploadPart is the only endpoint that treats this as
	// an error rather than an idempotent no-op).
	ErrAlreadyCompleted = errors.New("UploadAlreadyCompleted")

	// ErrIncomplete maps to the atproto "MissingParts" error, returned
	// by Finish when not all parts have been received yet.
	ErrIncomplete = errors.New("not all parts have been received")

	// ErrBadPartNumber maps to the atproto "InvalidPartNumber" error:
	// partNumber is out of the [1, partCount] range for the session.
	ErrBadPartNumber = errors.New("part number out of range")

	// ErrSizeMismatch maps to the atproto "PartSizeMismatch" error: the
	// bytes actually written for a part do not match the expected size
	// for that part number (partSizeBytes for all but the last part,
	// remainder for the last).
	ErrSizeMismatch = errors.New("uploaded part size does not match the expected size for this part")

	// ErrInvalidSize / ErrInvalidMimeType are returned by Start for
	// input that fails lexicon constraints.
	ErrInvalidSize     = errors.New("sizeBytes must be >= 1 (and within configured limits)")
	ErrInvalidMimeType = errors.New("mimeType must be between 3 and 255 characters")
)
