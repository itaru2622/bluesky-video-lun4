// Package bsky is a stand-in for the app.bsky.video.{startUpload,uploadPart,
// finishUpload,abortUpload,getUploadStatus} types that github.com/bluesky-social/indigo's
// api/bsky package does not generate yet (see internal/lexicons). File names
// and type names follow the same convention cmd/lexgen would produce, so
// this whole directory can be deleted and callers repointed at the real
// github.com/bluesky-social/indigo/api/bsky package once it catches up.
package bsky
