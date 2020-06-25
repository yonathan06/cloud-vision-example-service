package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	vision "cloud.google.com/go/vision/apiv1"
)

const maxUploadSize = 2 * 1024 // 2 MB

func main() {

	ctx := context.Background()

	client, err := vision.NewImageAnnotatorClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	http.HandleFunc("/imageml", http.HandlerFunc(higherOrderHandler(client)))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server started on localhost:%v", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func writeError(w http.ResponseWriter, message string, statusCode int) {
	w.WriteHeader(statusCode)
	w.Write([]byte(message))
}

func sendOptionsResponse(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:5500")
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

func checkAuthorization(w http.ResponseWriter, r *http.Request) bool {
	passes, ok := r.URL.Query()["pass"]
	if !ok || len(passes[0]) < 1 {
		writeError(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	pass := passes[0]

	if pass != "hackyourfuture20" {
		writeError(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	return true
}

func higherOrderHandler(client *vision.ImageAnnotatorClient) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sendOptionsResponse(w, r)
		if r.Method == "OPTIONS" {
			return
		}
		ok := checkAuthorization(w, r)
		if !ok {
			return
		}
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			fmt.Printf("Could not parse multipart form %v\n", err)
			writeError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		file, _, err := r.FormFile("image")
		if err != nil {
			fmt.Printf("Bad file %v\n", err)
			writeError(w, "Invalid File", http.StatusBadRequest)
			return
		}
		defer file.Close()

		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			writeError(w, "Invalid File", http.StatusBadRequest)
			return
		}

		fileType := http.DetectContentType(fileBytes)
		if fileType != "image/jpg" &&
			fileType != "image/jpeg" &&
			fileType != "image/png" &&
			fileType != "image/webp" {
			writeError(w, "Support only image/jpg image/jpeg image/webp & image/png", http.StatusBadRequest)
			return
		}

		tmpFile, err := ioutil.TempFile("/tmp", "")
		if err != nil {
			fmt.Printf("Error creating new file on disk %v\n", err)
			writeError(w, "Invalid File", http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmpFile.Name())

		tmpFile.Write(fileBytes)
		tmpFile.Close()

		ctx := context.Background()

		fileForImageVision, err := os.Open(tmpFile.Name())
		if err != nil {
			fmt.Printf("Failed to create image: %v\n", err)
			writeError(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		image, err := vision.NewImageFromReader(fileForImageVision)
		if err != nil {
			fmt.Printf("Failed to create image: %v\n", err)
			writeError(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		faces, err := client.DetectFaces(ctx, image, nil, 4)
		if err != nil {
			fmt.Printf("Failed to detect faces: %v\n", err)
			writeError(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		jsonData, err := json.Marshal(faces)

		if err != nil {
			fmt.Printf("Error parsing data to json: %v\n", err)
			writeError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.Write(jsonData)
	})

}
