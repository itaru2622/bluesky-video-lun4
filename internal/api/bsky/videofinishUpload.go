package bsky

import indigobsky "github.com/bluesky-social/indigo/api/bsky"

// VideoFinishUpload_Input is the input argument to a app.bsky.video.finishUpload call.
type VideoFinishUpload_Input struct {
	JobId string `json:"jobId"`
}

// VideoFinishUpload_Output is the output of a app.bsky.video.finishUpload call.
type VideoFinishUpload_Output struct {
	// CompletedJobId is the processing job to poll with getJobStatus; on
	// deduplication this may differ from the input jobId.
	CompletedJobId string                          `json:"completedJobId"`
	JobStatus      *indigobsky.VideoDefs_JobStatus `json:"jobStatus"`
}
