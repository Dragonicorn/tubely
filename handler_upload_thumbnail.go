package main

import (
	//"encoding/base64"
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

	// TODO: implement the upload here
	const maxMemory int64 = 10 << 20
	var Thumbnail thumbnail
	err = r.ParseMultipartForm(maxMemory)

	// "thumbnail" should match the HTML form input name
	// `file` is an `io.Reader` that we can read from to get the image data
	file, _, err := r.FormFile(("thumbnail"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	Thumbnail.data, err = io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read image data file", err)
		return
	}
	Thumbnail.mediaType = http.DetectContentType(Thumbnail.data)
	mediaType, _, err := mime.ParseMediaType(Thumbnail.mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse MIME type", err)
		return
	}
	//fmt.Printf("Content-Type = '%s', MIME Type = '%s'\n", Thumbnail.mediaType, mediaType)
	mediaType = strings.ToLower(mediaType)
	if mediaType == "image/jpeg" || mediaType == "image/png" {
		fmt.Printf("Content-Type = '%s', MIME Type = '%s'\n", Thumbnail.mediaType, mediaType)
	} else {
		respondWithError(w, http.StatusBadRequest, "Unable to upload MIME type as thumbnail", err)
		return
	}

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
		// store thumbnail in memory
			videoThumbnails[video.ID] = Thumbnail
			thumbnailURL := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, video.ID.String())
		// store thumbnail as base64 encoded data URL in database record for video
			base64Thumbnail := base64.StdEncoding.EncodeToString(Thumbnail.data)
			thumbnailURL := fmt.Sprintf("data:%s;base64,%s", Thumbnail.mediaType, base64Thumbnail)
	*/
	// store thumbnail in file system
	outExt, err := mime.ExtensionsByType(Thumbnail.mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid MIME content type", err)
		return
	}
	//fmt.Printf("Thumbnail file extension = %s\n", outExt[0])
	outName := make([]byte, 32)
	rand.Read(outName)
	outPath := filepath.Join(cfg.assetsRoot, base64.RawURLEncoding.EncodeToString(outName)+outExt[0])
	//outPath := filepath.Join(cfg.assetsRoot, video.ID.String()+outExt[0])
	fmt.Printf("Thumbnail file path = '%s'\n", outPath)
	err = os.WriteFile(outPath, Thumbnail.data, 0644)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to create thumbnail file", err)
		return
	}
	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s%s", cfg.port, base64.RawURLEncoding.EncodeToString(outName), outExt[0])
	video.ThumbnailURL = &thumbnailURL
	fmt.Printf("Video Thumbnail URL = %s\n", *video.ThumbnailURL)
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to update database record for video", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}
