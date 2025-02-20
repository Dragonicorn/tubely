package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	db               database.Client
	jwtSecret        string
	platform         string
	filepathRoot     string
	assetsRoot       string
	s3Bucket         string
	s3Region         string
	s3CfDistribution string
	s3Client         *s3.Client
	port             string
}

type thumbnail struct {
	data      []byte
	mediaType string
}

var ctx context.Context

var videoThumbnails = map[uuid.UUID]thumbnail{}

func processVideoForFastStart(filePath string) (string, error) {
	fmt.Printf("Input video file: %s\n", filePath)
	outPath := filePath + ".faststart"
	args := []string{"-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outPath}
	cmd := exec.Command("ffmpeg", args...)
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error executing ffmpeg command: %v", err)
		return "", err
	}
	fmt.Printf("Output video file: %s\n", outPath)
	return outPath, nil
}

func getVideoAspectRatio(filePath string) (string, error) {
	type streams struct {
		Streams []struct {
			Index  int    `json:"index"`
			Codec  string `json:"codec_type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		}
	}
	var out bytes.Buffer
	var fileStreams streams
	var w, h int
	var AR string

	//fmt.Printf("Video file: %s\n", filePath)

	args := []string{"-v", "error", "-print_format", "json", "-show_streams", filePath}
	cmd := exec.Command("ffprobe", args...)
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error executing ffprobe command: %v", err)
		return "", err
	}
	//fmt.Println("buffer:", out.String())

	err = json.Unmarshal(out.Bytes(), &fileStreams)
	if err != nil {
		return "", err
	}
	for _, stream := range fileStreams.Streams {
		if stream.Codec == "video" {
			w = stream.Width
			h = stream.Height
			//fmt.Printf("%s stream %d size = %d x %d\n", stream.Codec, stream.Index, w, h)
			break
		}
	}
	if (w == 0) || (h == 0) {
		fmt.Printf("Video size cannot have zero dimension\n")
		return "", nil
	}
	ar := float64(w) / float64(h)
	ar16_9 := float64(16) / float64(9)
	ar9_16 := float64(9) / float64(16)
	if ((ar16_9 * 0.9) < ar) && (ar < (ar16_9 * 1.1)) {
		AR = "16:9"
	} else if ((ar9_16 * 0.9) < ar) && (ar < (ar9_16 * 1.1)) {
		AR = "9:16"
	} else {
		AR = "other"
	}
	//fmt.Printf("Aspect Ratio: %f (%s)\n", ar, AR)

	return AR, nil
}

func main() {
	godotenv.Load(".env")

	pathToDB := os.Getenv("DB_PATH")
	if pathToDB == "" {
		log.Fatal("DB_URL must be set")
	}

	db, err := database.NewClient(pathToDB)
	if err != nil {
		log.Fatalf("Couldn't connect to database: %v", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is not set")
	}

	platform := os.Getenv("PLATFORM")
	if platform == "" {
		log.Fatal("PLATFORM environment variable is not set")
	}

	filepathRoot := os.Getenv("FILEPATH_ROOT")
	if filepathRoot == "" {
		log.Fatal("FILEPATH_ROOT environment variable is not set")
	}

	assetsRoot := os.Getenv("ASSETS_ROOT")
	if assetsRoot == "" {
		log.Fatal("ASSETS_ROOT environment variable is not set")
	}

	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		log.Fatal("S3_BUCKET environment variable is not set")
	}

	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		log.Fatal("S3_REGION environment variable is not set")
	}

	s3CfDistribution := os.Getenv("S3_CF_DISTRO")
	if s3CfDistribution == "" {
		log.Fatal("S3_CF_DISTRO environment variable is not set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT environment variable is not set")
	}

	ctx = context.TODO()
	s3Cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(s3Region))
	if err != nil {
		log.Fatalf("Couldn't load AWS configuration: %v", err)
	}
	cfg := apiConfig{
		db:               db,
		jwtSecret:        jwtSecret,
		platform:         platform,
		filepathRoot:     filepathRoot,
		assetsRoot:       assetsRoot,
		s3Bucket:         s3Bucket,
		s3Region:         s3Region,
		s3CfDistribution: s3CfDistribution,
		s3Client:         s3.NewFromConfig(s3Cfg),
		port:             port,
	}

	err = cfg.ensureAssetsDir()
	if err != nil {
		log.Fatalf("Couldn't create assets directory: %v", err)
	}

	mux := http.NewServeMux()
	appHandler := http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))
	mux.Handle("/app/", appHandler)

	assetsHandler := http.StripPrefix("/assets", http.FileServer(http.Dir(assetsRoot)))
	mux.Handle("/assets/", noCacheMiddleware(assetsHandler))

	mux.HandleFunc("POST /api/login", cfg.handlerLogin)
	mux.HandleFunc("POST /api/refresh", cfg.handlerRefresh)
	mux.HandleFunc("POST /api/revoke", cfg.handlerRevoke)

	mux.HandleFunc("POST /api/users", cfg.handlerUsersCreate)

	mux.HandleFunc("POST /api/videos", cfg.handlerVideoMetaCreate)
	mux.HandleFunc("POST /api/thumbnail_upload/{videoID}", cfg.handlerUploadThumbnail)
	mux.HandleFunc("POST /api/video_upload/{videoID}", cfg.handlerUploadVideo)
	mux.HandleFunc("GET /api/videos", cfg.handlerVideosRetrieve)
	mux.HandleFunc("GET /api/videos/{videoID}", cfg.handlerVideoGet)
	//mux.HandleFunc("GET /api/thumbnails/{videoID}", cfg.handlerThumbnailGet)
	mux.HandleFunc("DELETE /api/videos/{videoID}", cfg.handlerVideoMetaDelete)

	mux.HandleFunc("POST /admin/reset", cfg.handlerReset)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Serving on: http://localhost:%s/app/\n", port)
	log.Fatal(srv.ListenAndServe())
}
