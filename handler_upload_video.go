package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// set upload limit of 1GB
	const maxMemory int64 = 1 << 30
	var keyStr string
	var Video thumbnail

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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Unable to get database record for video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You can't upload this video", err)
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	err = r.ParseMultipartForm(maxMemory)

	// "video" should match the HTML form input name
	// `file` is an `io.Reader` that we can read from to get the image data
	file, _, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	Video.data, err = io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read video data file", err)
		return
	}
	Video.mediaType = http.DetectContentType(Video.data)
	mediaType, _, err := mime.ParseMediaType(Video.mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse MIME type", err)
		return
	}
	mediaType = strings.ToLower(mediaType)
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Unable to upload MIME type as video", err)
		return
	}
	//fmt.Printf("Content-Type = '%s', MIME Type = '%s'\n", Video.mediaType, mediaType)

	tmp, err := os.CreateTemp("", "tubely-upload*.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to create temporary file", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	err = os.WriteFile(tmp.Name(), Video.data, 0644)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to create video file", err)
		return
	}
	fsVideo, err := processVideoForFastStart(tmp.Name())
	defer os.Remove(fsVideo)
	fs, err := os.Open(fsVideo)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to create fast start video file", err)
		return
	}
	defer fs.Close()
	ar, err := getVideoAspectRatio(fsVideo)
	switch ar {
	case "16:9":
		keyStr = "landscape"
	case "9:16":
		keyStr = "portrait"
	default:
		keyStr = "other"
	}
	fmt.Printf("%s returned from getVideoAspectRatio\nUsing %s prefix for AWS\n", ar, keyStr)
	_, err = fs.Seek(0, 0)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to seek to beginning of fast video file", err)
		return
	}
	key := make([]byte, 32)
	rand.Read(key)
	keyStr += "/" + base64.RawURLEncoding.EncodeToString(key) + ".mp4"
	_, err = cfg.s3Client.PutObject(ctx, &s3.PutObjectInput{Bucket: &cfg.s3Bucket, Key: &keyStr, Body: fs, ContentType: &mediaType})
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to upload video file to AWS", err)
		return
	}
	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, keyStr)
	video.VideoURL = &videoURL
	fmt.Printf("Video URL = %s\n", *video.VideoURL)
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to update database record for video", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}
