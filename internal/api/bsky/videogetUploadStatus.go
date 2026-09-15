package bsky

import (
	"time"

	indigobsky "github.com/bluesky-social/indigo/api/bsky"
)

// VideoGetUploadStatus_Output is the output of a app.bsky.video.getUploadStatus call.
type VideoGetUploadStatus_Output struct {
	JobId string `json:"jobId"`
	// State is one of: created | finishing | completed | failed |
	// aborted | expired.
	State         string    `json:"state"`
	PartSizeBytes int64     `json:"partSizeBytes"`
	PartCount     int64     `json:"partCount"`
	ReceivedParts []int64   `json:"receivedParts"`
	ExpiresAt     time.Time `json:"expiresAt"`
	// CompletedJobId/JobStatus are present only when state == "completed".
	CompletedJobId *string                         `json:"completedJobId,omitempty"`
	JobStatus      *indigobsky.VideoDefs_JobStatus `json:"jobStatus,omitempty"`
	// FailureReason is present only when state == "failed".
	FailureReason *string `json:"failureReason,omitempty"`
}
