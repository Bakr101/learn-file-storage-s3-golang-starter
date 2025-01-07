package main

import (
	"fmt"
	"io"
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

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)
	file,header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get form file", err)
		return
	}
	defer file.Close()
	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for thumbnail", nil)
		return
	}
	
	// data, err := io.ReadAll(file)
	// if err != nil {
	// 	respondWithError(w, http.StatusInternalServerError, "Couldn't read img data", err)
	// 	return
	// }
	videoMetadata,err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't get video data", err)
		return
	}
	if videoMetadata.UserID != userID{
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access", err)
		return
	}
	imgType := strings.Split(mediaType, "/")[1]
	videoNameExt:= videoIDString + "." + imgType 
	URLPath := filepath.Join(cfg.assetsRoot,videoNameExt)
	assetfile, err := os.Create(URLPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't create file", err)
		return
	}
	_, err = io.Copy(assetfile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't copy to file", err)
		return
	}
	dataURL := fmt.Sprintf("http://localhost:%s/%s", cfg.port, URLPath)
	videoMetadata.ThumbnailURL = &dataURL
	
	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMetadata)
}
