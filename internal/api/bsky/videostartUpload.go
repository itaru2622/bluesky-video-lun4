package bsky

import "time"

// VideoStartUpload_Input is the input argument to a app.bsky.video.startUpload call.
type VideoStartUpload_Input struct {
	// MimeType is the declared MIME type of the video. len 3..255.
	MimeType string `json:"mimeType"`
	// SizeBytes is the exact byte size of the complete upload-ready
	// video file before it is split into parts. Must be >= 1.
	SizeBytes int64 `json:"sizeBytes"`
	// Name is an optional client-provided file name. len <= 256.
	Name *string `json:"name,omitempty"`
	// DurationMs/Height/Width are advisory, non-authoritative and used
	// only for early failure; the authoritative probe runs
	// asynchronously after upload.
	DurationMs *int64 `json:"durationMs,omitempty"`
	Height     *int64 `json:"height,omitempty"`
	Width      *int64 `json:"width,omitempty"`
}

// VideoStartUpload_Output is the output of a app.bsky.video.startUpload call.
type VideoStartUpload_Output struct {
	JobId         string    `json:"jobId"`
	PartSizeBytes int64     `json:"partSizeBytes"`
	PartCount     int64     `json:"partCount"`
	ExpiresAt     time.Time `json:"expiresAt"`
}
