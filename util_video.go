package main

import (
	"encoding/json"
	"os/exec"
)

// ffprobe result structure in json.
type ffProbeResult struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

// codec info
type codecInfo struct{ Video, Audio string }


func getCodec(videoPath string)  (codecInfo, error) {
// get codecs of video and audio for videoPath

	probeCmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name", "-print_format", "json", videoPath,	)

	probeOutput, err := probeCmd.Output()
	if err != nil {
		return codecInfo{}, err // error on process
	}

	var result ffProbeResult
	if err := json.Unmarshal(probeOutput, &result); err != nil {
		return codecInfo{}, err // error on parse result
	}

	var rtn codecInfo
	for _, s := range result.Streams {
		switch s.CodecType {
			case "video":
				if rtn.Video == "" { rtn.Video = s.CodecName }
			case "audio":
				if rtn.Audio == "" { rtn.Audio = s.CodecName }
		}
	}

	return rtn, nil
}

func shouldCopyIntoHLS(videoPath string) bool {
// decide convert mode,  stream-copy or content re-encoding for saving time.
// returns true when video=H.264 and audio=AAC, false otherwise.

	ci, err := getCodec(videoPath)
	if err != nil {
		return false
	}

	return (ci.Video == "h264" && ci.Audio == "aac")
}
