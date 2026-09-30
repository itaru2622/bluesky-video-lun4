package main

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
)

// ffprobe result structure in json.
type ffProbeResult struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Tags      struct {
			// Legacy MP4 rotation tag (e.g. "90", "180", "270"), as set
			// by older recording apps/devices.
			Rotate string `json:"rotate"`
		} `json:"tags"`
		// Newer encoders express rotation as a "Display Matrix" side
		// data entry instead of (or alongside) the rotate tag above.
		SideDataList []struct {
			SideDataType string  `json:"side_data_type"`
			Rotation     float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
}

// stream info
type streamInfo struct {
	VideoCodec, AudioCodec string

	// HasRotation is true when the video stream carries rotation
	// metadata (legacy "rotate" tag, or a "Display Matrix" side data
	// entry with a non-zero angle).
	// When rotated, it needs to re-encode but NOT stream-copy,
	// since stream-copy doesnot have rotation consideration.
	//
	// MPEG-TS (and therefore HLS, which segments MPEG-TS) has no field
	// equivalent to MP4's rotate tag/display matrix, and ffmpeg's HLS
	// muxer does not carry even generic mpegts-level metadata through to
	// segments. So a stream-copy of a video with rotation metadata would
	// silently produce a sideways/upside-down HLS stream, with ffmpeg
	// exiting 0 (no error to catch). Re-encoding is the only reliable
	// fix: ffmpeg's decoder applies the rotation to the decoded pixels
	// before the encoder ever sees them (autorotate, on by default),
	// baking the correct orientation in regardless of container.
	HasRotation bool
}

func getStreamInfo(videoPath string) (streamInfo, error) {
	// get codecs of video and audio for videoPath, plus whether the
	// video stream carries rotation metadata.

	probeCmd := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "stream=codec_type,codec_name:stream_tags=rotate:stream_side_data=rotation",
		"-print_format", "json", videoPath,
	)

	probeOutput, err := probeCmd.Output()
	if err != nil {
		return streamInfo{}, err // error on process
	}

	var result ffProbeResult
	if err := json.Unmarshal(probeOutput, &result); err != nil {
		return streamInfo{}, err // error on parse result
	}

	var rtn streamInfo
	for _, s := range result.Streams {
		switch s.CodecType {
		case "audio":
			if rtn.AudioCodec == "" {
				rtn.AudioCodec = s.CodecName
			}
		case "video":
			if rtn.VideoCodec == "" {
				rtn.VideoCodec = s.CodecName
			}

			if r := strings.TrimSpace(s.Tags.Rotate); r != "" {
				if deg, err := strconv.Atoi(r); err == nil && deg%360 != 0 {
					rtn.HasRotation = true
				}
			}

			for _, sd := range s.SideDataList {
				if sd.SideDataType == "Display Matrix" && sd.Rotation != 0 {
					rtn.HasRotation = true
				}
			}
		}
	}

	return rtn, nil
}

func shouldCopyIntoHLS(videoPath string) bool {
	// decide convert mode, stream-copy or content re-encoding for saving time.
	// returns true when video=H.264, audio=AAC, and the video carries no
	// rotation metadata; false otherwise (including on probe failure,
	// which conservatively falls back to re-encoding).

	si, err := getStreamInfo(videoPath)
	if err != nil {
		return false
	}

	return ( si.VideoCodec == "h264" && si.AudioCodec == "aac" && !si.HasRotation )
}
