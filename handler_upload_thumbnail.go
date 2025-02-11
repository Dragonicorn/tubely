package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

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

	// TODO: implement the upload here
	const maxMemory int64 = 10 << 20
	var Thumbnail thumbnail
	err = r.ParseMultipartForm(maxMemory)

	// "thumbnail" should match the HTML form input name
	file, _, err := r.FormFile(("thumbnail"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// `file` is an `io.Reader` that we can read from to get the image data
	Thumbnail.data, err = io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read image data file", err)
		return
	}
	Thumbnail.mediaType = http.DetectContentType(Thumbnail.data)
	fmt.Printf("Content-Type = %s\n", Thumbnail.mediaType)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Unable to get database record for video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You can't modify this video", err)
		return
	}
	/*
		videoThumbnails[video.ID] = Thumbnail
		thumbnailURL := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, video.ID.String())
	*/
	// store thumbnail as base64 encoded data URL in database record for video
	base64Thumbnail := base64.StdEncoding.EncodeToString(Thumbnail.data)
	thumbnailURL := fmt.Sprintf("data:%s;base64,%s", Thumbnail.mediaType, base64Thumbnail)
	video.ThumbnailURL = &thumbnailURL
	// fmt.Printf("Video Thumbnail URL = %s\n", *video.ThumbnailURL)
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to update database record for video", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}
