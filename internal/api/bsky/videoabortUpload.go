package bsky

// VideoAbortUpload_Input is the input argument to a app.bsky.video.abortUpload call.
type VideoAbortUpload_Input struct {
	JobId string `json:"jobId"`
}

// VideoAbortUpload_Output is the output of a app.bsky.video.abortUpload call.
type VideoAbortUpload_Output struct {
	// State is one of: aborted | completed | failed | expired.
	State          string  `json:"state"`
	CompletedJobId *string `json:"completedJobId,omitempty"`
	// FailureReason is present only when state == "failed".
	FailureReason *string `json:"failureReason,omitempty"`
}
