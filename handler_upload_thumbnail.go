package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const MaxMemory = 10 << 20

	r.ParseMultipartForm(MaxMemory)

	data, hPtr, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}
	defer data.Close()

	conType := hPtr.Header.Get("Content-Type")
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "video not found", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "video does not belong to user", nil)
		return
	}

	mediaType, _, err := mime.ParseMediaType(conType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "error parsing media type", err)
		return
	}

	if mediaType != "image/png" && mediaType != "image/jpg" {
		respondWithError(w, http.StatusBadRequest, "invalid file format, thumbnails must be either png or jpg filetypes", err)

	}

	randData := make([]byte, 32)
	_, err = rand.Read(randData)
	if err != nil {
		respondWithError(w, 500, "error creating filename", err)
		return
	}

	randName := base64.RawURLEncoding.EncodeToString(randData)
	filename := randName + "." + strings.TrimPrefix(conType, "image/")
	filePath := filepath.Join(cfg.assetsRoot, filename)
	file, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, 500, "error creating file", err)
		return
	}
	defer file.Close()

	_, err = io.Copy(file, data)
	if err != nil {
		respondWithError(w, 500, "error copying image data", err)
		return
	}

	thumbnailURL := "http://localhost:8091/assets/" + filename
	video.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error updating video in database", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
