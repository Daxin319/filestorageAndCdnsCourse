package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

type videoStreams struct {
	Streams []struct {
		Index              int    `json:"index"`
		CodecName          string `json:"codec_name,omitempty"`
		CodecLongName      string `json:"codec_long_name,omitempty"`
		Profile            string `json:"profile,omitempty"`
		CodecType          string `json:"codec_type"`
		CodecTagString     string `json:"codec_tag_string"`
		CodecTag           string `json:"codec_tag"`
		Width              int    `json:"width,omitempty"`
		Height             int    `json:"height,omitempty"`
		CodedWidth         int    `json:"coded_width,omitempty"`
		CodedHeight        int    `json:"coded_height,omitempty"`
		ClosedCaptions     int    `json:"closed_captions,omitempty"`
		FilmGrain          int    `json:"film_grain,omitempty"`
		HasBFrames         int    `json:"has_b_frames,omitempty"`
		SampleAspectRatio  string `json:"sample_aspect_ratio,omitempty"`
		DisplayAspectRatio string `json:"display_aspect_ratio,omitempty"`
		PixFmt             string `json:"pix_fmt,omitempty"`
		Level              int    `json:"level,omitempty"`
		ColorRange         string `json:"color_range,omitempty"`
		ColorSpace         string `json:"color_space,omitempty"`
		ColorTransfer      string `json:"color_transfer,omitempty"`
		ColorPrimaries     string `json:"color_primaries,omitempty"`
		ChromaLocation     string `json:"chroma_location,omitempty"`
		FieldOrder         string `json:"field_order,omitempty"`
		Refs               int    `json:"refs,omitempty"`
		IsAvc              string `json:"is_avc,omitempty"`
		NalLengthSize      string `json:"nal_length_size,omitempty"`
		ID                 string `json:"id"`
		RFrameRate         string `json:"r_frame_rate"`
		AvgFrameRate       string `json:"avg_frame_rate"`
		TimeBase           string `json:"time_base"`
		StartPts           int    `json:"start_pts"`
		StartTime          string `json:"start_time"`
		DurationTs         int    `json:"duration_ts"`
		Duration           string `json:"duration"`
		BitRate            string `json:"bit_rate,omitempty"`
		BitsPerRawSample   string `json:"bits_per_raw_sample,omitempty"`
		NbFrames           string `json:"nb_frames"`
		ExtradataSize      int    `json:"extradata_size"`
		Disposition        struct {
			Default         int `json:"default"`
			Dub             int `json:"dub"`
			Original        int `json:"original"`
			Comment         int `json:"comment"`
			Lyrics          int `json:"lyrics"`
			Karaoke         int `json:"karaoke"`
			Forced          int `json:"forced"`
			HearingImpaired int `json:"hearing_impaired"`
			VisualImpaired  int `json:"visual_impaired"`
			CleanEffects    int `json:"clean_effects"`
			AttachedPic     int `json:"attached_pic"`
			TimedThumbnails int `json:"timed_thumbnails"`
			NonDiegetic     int `json:"non_diegetic"`
			Captions        int `json:"captions"`
			Descriptions    int `json:"descriptions"`
			Metadata        int `json:"metadata"`
			Dependent       int `json:"dependent"`
			StillImage      int `json:"still_image"`
		} `json:"disposition"`
		Tags struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			VendorID    string `json:"vendor_id"`
			Encoder     string `json:"encoder"`
			Timecode    string `json:"timecode"`
		} `json:"tags,omitempty"`
		SampleFmt      string `json:"sample_fmt,omitempty"`
		SampleRate     string `json:"sample_rate,omitempty"`
		Channels       int    `json:"channels,omitempty"`
		ChannelLayout  string `json:"channel_layout,omitempty"`
		BitsPerSample  int    `json:"bits_per_sample,omitempty"`
		InitialPadding int    `json:"initial_padding,omitempty"`
		Tags0          struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			VendorID    string `json:"vendor_id"`
		} `json:"tags,omitempty"`
		Tags1 struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			Timecode    string `json:"timecode"`
		} `json:"tags,omitempty"`
	} `json:"streams"`
}

func check(w http.ResponseWriter, code int, msg string, err error) bool {
	if err != nil {
		respondWithError(w, code, msg, err)
		return true
	}
	return false
}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(1<<30))

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if check(w, http.StatusBadRequest, "Invalid video ID", err) {
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if check(w, http.StatusUnauthorized, "Couldn't find JWT", err) {
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if check(w, http.StatusUnauthorized, "Couldn't validate JWT", err) {
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if check(w, http.StatusNotFound, "video not found", err) {
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "video does not belong to this user", err)
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	data, hPtr, err := r.FormFile("video")
	if check(w, http.StatusBadRequest, "invalid file type", err) {
		return
	}
	defer data.Close()

	conType := hPtr.Header.Get("Content-Type")

	mediaType, _, err := mime.ParseMediaType(conType)
	if check(w, http.StatusBadRequest, "error parsing media type", err) {
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "invalid video file type, videos must be a .mp4", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if check(w, http.StatusInternalServerError, "error creating temp file", err) {
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, data)
	if check(w, http.StatusInternalServerError, "error copying file data", err) {
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if check(w, http.StatusInternalServerError, "error resetting file pointer", err) {
		return
	}

	tempPath, err := processVideoForFastStart(tempFile.Name())
	if check(w, http.StatusInternalServerError, "error processing video for fast start", err) {
		return
	}

	tempVideo, err := os.Open(tempPath)
	if check(w, http.StatusInternalServerError, "error opening processed video", err) {
		return
	}
	defer os.Remove(tempVideo.Name())
	defer tempVideo.Close()

	_, err = tempVideo.Seek(0, io.SeekStart)
	if check(w, http.StatusInternalServerError, "error resetting file pointer in processed video", err) {
		return
	}

	randData := make([]byte, 32)
	_, err = rand.Read(randData)
	if check(w, 500, "error creating filename", err) {
		return
	}

	randName := base64.RawURLEncoding.EncodeToString(randData)
	ratio, err := getVideoAspectRatio(tempVideo.Name())
	if check(w, http.StatusInternalServerError, "error getting video aspect ratio", err) {
		return
	}

	var filename string
	switch ratio {
	case "9:16":
		filename = "landscape/" + randName + "." + strings.TrimPrefix(conType, "video/")
	case "16:9":
		filename = "portrait/" + randName + "." + strings.TrimPrefix(conType, "video/")
	case "other":
		filename = "other/" + randName + "." + strings.TrimPrefix(conType, "video/")
	}

	if video.VideoURL != nil {

		oldKey := strings.TrimPrefix(*video.VideoURL, cfg.s3Bucket+",")
		oldKey = strings.TrimPrefix(*video.VideoURL, "https://"+cfg.s3Bucket+".s3."+cfg.s3Region+".amazonaws.com/")
		oldKey = strings.TrimPrefix(*video.VideoURL, cfg.s3CfDistribution+"/")

		args := &s3.DeleteObjectInput{
			Bucket: &cfg.s3Bucket,
			Key:    &oldKey,
		}

		_, err = cfg.s3Client.DeleteObject(context.Background(), args)
		if check(w, http.StatusInternalServerError, "error deleting old file", err) {
			return
		}
	}

	input := &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &filename,
		Body:        tempVideo,
		ContentType: &mediaType,
	}

	_, err = cfg.s3Client.PutObject(context.Background(), input)
	if check(w, http.StatusInternalServerError, "error uploading file to s3", err) {
		return
	}

	videoUrl := cfg.s3CfDistribution + "/" + filename
	video.VideoURL = &videoUrl

	err = cfg.db.UpdateVideo(video)
	if check(w, http.StatusInternalServerError, "error updating video URL in database", err) {
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	var streams videoStreams
	stdout := buffer.Bytes()
	err = json.Unmarshal(stdout, &streams)
	if err != nil {
		return "", err
	}
	ratio := int(streams.Streams[0].Height) / int(streams.Streams[0].Width)
	landscape := 9 / 16
	portrait := 16 / 9
	if ratio == landscape {
		return "9:16", nil
	}
	if ratio == portrait {
		return "16:9", nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	tempPath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", tempPath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return tempPath, nil
}
