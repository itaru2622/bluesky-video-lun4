package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/gin-gonic/gin"
	gonanoid "github.com/matoous/go-nanoid"

	bskyshim "github.com/itaru2622/bluesky-video-lun4/internal/api/bsky"
	"github.com/itaru2622/bluesky-video-lun4/internal/videoupload"
)

// respondXRPCError writes an atproto-style XRPC error body
// ({"error": "<Name>", "message": "..."}) and aborts the gin context, so
// clients get the same shape they'd get from a real PDS/AppView error.
func respondXRPCError(c *gin.Context, status int, name, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": name, "message": message})
}

// videoUploadErrorStatus maps a videoupload sentinel error onto an HTTP
// status + atproto XRPC error name.
func videoUploadErrorStatus(err error) (status int, name string) {
	switch err {
	case videoupload.ErrNotFound:
		return http.StatusNotFound, "UploadNotFound"
	case videoupload.ErrExpired:
		return http.StatusGone, "UploadExpired"
	case videoupload.ErrUploadNotReady:
		return http.StatusConflict, "UploadNotReady"
	case videoupload.ErrUploadFailed:
		return http.StatusConflict, "UploadFailed"
	case videoupload.ErrUploadAborted:
		return http.StatusConflict, "UploadAborted"
	case videoupload.ErrAlreadyCompleted:
		return http.StatusConflict, "UploadAlreadyCompleted"
	case videoupload.ErrIncomplete:
		return http.StatusBadRequest, "MissingParts"
	case videoupload.ErrBadPartNumber:
		return http.StatusBadRequest, "InvalidPartNumber"
	case videoupload.ErrSizeMismatch:
		return http.StatusBadRequest, "PartSizeMismatch"
	case videoupload.ErrInvalidSize, videoupload.ErrInvalidMimeType:
		return http.StatusBadRequest, "InvalidRequest"
	default:
		return http.StatusInternalServerError, "InternalServerError"
	}
}

// finishVideoUpload is plugged into state.videoMgr as its
// videoupload.FinishFunc. It hands the fully-assembled video off to
// exactly the same PDS-upload pipeline the pre-existing (single-shot)
// uploadVideo endpoint already uses (mint a Job, store it, process it in
// the background), so both upload paths share one job/state machine and
// one implementation of "talk to the user's PDS".
func (s *State) finishVideoUpload(userDID, token string, video *os.File, mimeType, name string) (string, *bsky.VideoDefs_JobStatus, error) {
	body, err := io.ReadAll(video)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read assembled video: %w", err)
	}

	jobID := gonanoid.MustGenerate("abcdefghimnopqrstuvwxyz1234567890", 10)
	job := Job{
		ID:          jobID,
		userDID:     userDID,
		state:       "processing",
		progress:    1,
		token:       token,
		contentType: mimeType,
	}
	s.jobs.Store(jobID, job)
	go s.process(job, body)

	return jobID, job.ToBsky(), nil
}

// startUpload implements POST /xrpc/app.bsky.video.startUpload.
func (s *State) startUpload(c *gin.Context) {
	userDID := c.GetString("user_did")
	if len(s.allowedDIDs) > 0 && !slices.Contains(s.allowedDIDs, userDID) {
		respondXRPCError(c, http.StatusForbidden, "InvalidRequest", "DID not allowed")
		return
	}

	var in bskyshim.VideoStartUpload_Input
	if err := c.ShouldBindJSON(&in); err != nil {
		respondXRPCError(c, http.StatusBadRequest, "InvalidRequest", err.Error())
		return
	}

	out, err := s.videoMgr.Start(userDID, c.GetHeader("authorization"), in)
	if err != nil {
		status, name := videoUploadErrorStatus(err)
		respondXRPCError(c, status, name, err.Error())
		return
	}
	c.JSON(http.StatusOK, out)
}

// uploadPart implements POST /xrpc/app.bsky.video.uploadPart.
// jobId/partNumber arrive as query params; the request body is the raw
// part bytes (Content-Type is opaque here, same as com.atproto.repo.uploadBlob).
func (s *State) uploadPart(c *gin.Context) {
	jobID := c.Query("jobId")
	partNumber, err := strconv.ParseInt(c.Query("partNumber"), 10, 64)
	if err != nil || partNumber < 1 {
		respondXRPCError(c, http.StatusBadRequest, "InvalidRequest", "partNumber must be a positive integer")
		return
	}

	// c.Request.ContentLength is -1 when unknown (e.g. chunked
	// transfer-encoding); WritePart skips the pre-check in that case
	// and instead relies on the read-one-byte-past-expected guard.
	out, err := s.videoMgr.WritePart(jobID, partNumber, c.Request.ContentLength, c.Request.Body)
	if err != nil {
		status, name := videoUploadErrorStatus(err)
		respondXRPCError(c, status, name, err.Error())
		return
	}
	c.JSON(http.StatusOK, out)
}

// finishUpload implements POST /xrpc/app.bsky.video.finishUpload.
func (s *State) finishUpload(c *gin.Context) {
	var in bskyshim.VideoFinishUpload_Input
	if err := c.ShouldBindJSON(&in); err != nil {
		respondXRPCError(c, http.StatusBadRequest, "InvalidRequest", err.Error())
		return
	}

	out, err := s.videoMgr.Finish(in.JobId)
	if err != nil {
		status, name := videoUploadErrorStatus(err)
		respondXRPCError(c, status, name, err.Error())
		return
	}
	c.JSON(http.StatusOK, out)
}

// abortUpload implements POST /xrpc/app.bsky.video.abortUpload.
func (s *State) abortUpload(c *gin.Context) {
	var in bskyshim.VideoAbortUpload_Input
	if err := c.ShouldBindJSON(&in); err != nil {
		respondXRPCError(c, http.StatusBadRequest, "InvalidRequest", err.Error())
		return
	}

	out, err := s.videoMgr.Abort(in.JobId)
	if err != nil {
		status, name := videoUploadErrorStatus(err)
		respondXRPCError(c, status, name, err.Error())
		return
	}
	c.JSON(http.StatusOK, out)
}

// getUploadStatus implements GET /xrpc/app.bsky.video.getUploadStatus.
func (s *State) getUploadStatus(c *gin.Context) {
	out, err := s.videoMgr.Status(c.Query("jobId"))
	if err != nil {
		status, name := videoUploadErrorStatus(err)
		respondXRPCError(c, status, name, err.Error())
		return
	}
	c.JSON(http.StatusOK, out)
}
