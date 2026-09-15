package bsky

// VideoUploadPart_Output is the output of a app.bsky.video.uploadPart call.
// (There is no JSON input type: the request body is the raw part bytes;
// jobId/partNumber travel as query parameters.)
type VideoUploadPart_Output struct {
	PartNumber int64 `json:"partNumber"`
	SizeBytes  int64 `json:"sizeBytes"`
}
