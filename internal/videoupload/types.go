// Package videoupload implements the server-side session state machine for
// the chunked video upload endpoints introduced by
// https://github.com/bluesky-social/atproto/pull/5384:
//
//	app.bsky.video.startUpload
//	app.bsky.video.uploadPart
//	app.bsky.video.finishUpload
//	app.bsky.video.abortUpload
//	app.bsky.video.getUploadStatus
//
// These lexicons are not yet vendored into bluesky-social/indigo's
// api/bsky package, so their request/response types live in
// internal/api/bsky instead (see that package's doc comment). This
// package only holds the session state machine and error types, which
// are specific to this server and unaffected by indigo eventually
// catching up.
package videoupload
