package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

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

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)
	file,header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "image/jpeg" && mediaType != "image/png"{
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}
	
	
	assetPath := getAssetPath( mediaType)
	assetDiskPath := cfg.getAssetDiskPath(assetPath)
	assetfile, err := os.Create(assetDiskPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't create file", err)
		return
	}
	defer assetfile.Close()

	_, err = io.Copy(assetfile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't copy to file", err)
		return
	}
	
	videoMetadata,err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video data", err)
		return
	}
	if videoMetadata.UserID != userID{
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access", err)
		return
	}
	
	assetURL := cfg.getAssetURL(assetPath)
	videoMetadata.ThumbnailURL = &assetURL
	
	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMetadata)
}
