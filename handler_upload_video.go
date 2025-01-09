package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
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


	fmt.Println("uploading video", videoID, "by user", userID)
	
	videoMetadata,err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video data", err)
		return
	}
	if videoMetadata.UserID != userID{
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access", err)
		return
	}

	//upload limit
	const maxUpload = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	file,handler, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(handler.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4"{
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	
	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't copy to file", err)
		return
	}
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "Couldn't reset temp file ptr", err)
		return
	}
	
	assetPath := getAssetPath(mediaType)
	input := &s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &assetPath,
		Body: tempFile,
		ContentType: &mediaType,
	}

	_, err = cfg.s3Client.PutObject(r.Context(), input)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload to s3", err)
		return
	}
	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, assetPath)
	videoMetadata.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "couldnt update video in DB", err)
		return
	}
	respondWithJSON(w, http.StatusOK, videoMetadata)
}
