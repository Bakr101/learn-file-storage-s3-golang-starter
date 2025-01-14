package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

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
	//Fast start for streaming
	processedVideoPath, err := processVideoForFastStart(tempFile.Name())
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video", err)
		return
	}
	defer os.Remove(processedVideoPath)
	processedVideo, err := os.Open(processedVideoPath)
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "Couldn't find processed video", err)
		return
	}
	defer processedVideo.Close()
	//Aspect Ratio from temp 
	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video Aspect Ratio", err)
		return
	}
	
	assetPath := fmt.Sprintf("%v/%v", aspectRatio, getAssetPath(mediaType))
	input := &s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &assetPath,
		Body: processedVideo,
		ContentType: &mediaType,
	}

	_, err = cfg.s3Client.PutObject(r.Context(), input)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload to s3", err)
		return
	}
	//Bucket & key with comma delimeter
	videoURL := fmt.Sprintf("%s/%s", cfg.s3CfDistribution, assetPath)
	videoMetadata.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "couldnt update video in DB", err)
		return
	}
	// signedVideo, err := cfg.dbVideoToSignedVideo(videoMetadata)
	// if err != nil {
	// 	respondWithError(w, http.StatusUnauthorized, "couldnt access video", err)
	// 	return
	// }
	respondWithJSON(w, http.StatusOK, videoMetadata)
}


func getVideoAspectRatio(filePath string) (string, error){
	command := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	stdOutput  := &bytes.Buffer{}
	command.Stdout = stdOutput
	err := command.Run()
	if err != nil {
		return "", err
	}

	ffprobeOutput := ffprobeOutput{}
	err = json.Unmarshal(stdOutput.Bytes(), &ffprobeOutput)
	if err != nil {
		return "", err
	}
	
	streams := ffprobeOutput.Streams[0]
	aspectRatio, err := aspectRatioCalculator(streams.Width, streams.Height)
	if err != nil {
		return "", err
	}
	if aspectRatio == "16:9"{
		
		aspectRatio = "landscape"
	}else if aspectRatio == "9:16"{
		
		aspectRatio = "portrait"
	}
	
	return aspectRatio, nil
}


func processVideoForFastStart(filePath string) (string, error){
	processedVideoPath := fmt.Sprintf("%s.processing", filePath)
	command := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", processedVideoPath)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	err := command.Run()
	if err != nil {
		return "", err
	}
	//check file
	fileInfo, err := os.Stat(processedVideoPath)
	if err != nil {
		return "", fmt.Errorf("could not stat processed file: %v", err)
	}
	if fileInfo.Size() == 0 {
		return "", fmt.Errorf("processed file is empty")
	}
	return processedVideoPath, nil
}

// func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error){
// 	preSignClient := s3.NewPresignClient(s3Client)
// 	timeFunc := s3.WithPresignExpires(expireTime)
// 	req, err := preSignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
// 		Bucket: &bucket,
// 		Key: &key,
// 	}, timeFunc)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to generate presigned URL: %v", err)
// 	}
// 	return req.URL, nil
// }

// func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error){
// 	//Bucket & key
// 	if video.VideoURL == nil {
// 		return video, nil
// 	}
	
// 	URLSLice := strings.Split(*video.VideoURL, ",")
// 	if len(URLSLice) < 2{
// 		return video, nil
// 	}
	
// 	signedURL, err := generatePresignedURL(cfg.s3Client, URLSLice[0], URLSLice[1], 1 * time.Hour)
// 	if err != nil {
// 		return video, err
// 	}
// 	video.VideoURL = &signedURL
// 	return video, nil
// }