package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type splitResponse struct {
	RunID      string   `json:"run_id"`
	Chunks     []string `json:"chunks"`
	DurationMS int64    `json:"duration_ms"`
}

func main() {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}
	s3c := s3.NewFromConfig(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/split", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		input := r.URL.Query().Get("input")
		if input == "" {
			http.Error(w, "missing query param: input (expected s3://bucket/key)", http.StatusBadRequest)
			return
		}

		inBucket, inKey, err := parseS3URL(input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		outBucket := r.URL.Query().Get("out_bucket")
		if outBucket == "" {
			outBucket = inBucket
		}
		outPrefix := r.URL.Query().Get("out_prefix")
		if outPrefix == "" {
			outPrefix = "chunks"
		}
		outPrefix = strings.Trim(outPrefix, "/")

		reqCtx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		obj, err := s3c.GetObject(reqCtx, &s3.GetObjectInput{
			Bucket: &inBucket,
			Key:    &inKey,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("GetObject failed: %v", err), http.StatusBadRequest)
			return
		}
		defer obj.Body.Close()

		data, err := io.ReadAll(obj.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("read body failed: %v", err), http.StatusInternalServerError)
			return
		}

		parts := splitIntoThree(data)
		runID := fmt.Sprintf("%d", time.Now().UTC().UnixNano())

		chunkURLs := make([]string, 0, 3)
		for i, p := range parts {
			key := fmt.Sprintf("%s/%s/chunk-%d.txt", outPrefix, runID, i+1)
			_, err := s3c.PutObject(reqCtx, &s3.PutObjectInput{
				Bucket:      &outBucket,
				Key:         &key,
				Body:        bytes.NewReader(p),
				ContentType: strPtr("text/plain; charset=utf-8"),
			})
			if err != nil {
				http.Error(w, fmt.Sprintf("PutObject failed: %v", err), http.StatusInternalServerError)
				return
			}
			chunkURLs = append(chunkURLs, fmt.Sprintf("s3://%s/%s", outBucket, key))
		}

		resp := splitResponse{
			RunID:      runID,
			Chunks:     chunkURLs,
			DurationMS: time.Since(start).Milliseconds(),
		}
		writeJSON(w, resp)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("splitter listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func parseS3URL(s string) (bucket, key string, err error) {
	if !strings.HasPrefix(s, "s3://") {
		return "", "", errors.New("input must start with s3://")
	}
	rest := strings.TrimPrefix(s, "s3://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid s3 url, expected s3://bucket/key")
	}
	return parts[0], parts[1], nil
}

func splitIntoThree(data []byte) [][]byte {
	n := len(data)
	if n == 0 {
		return [][]byte{[]byte{}, []byte{}, []byte{}}
	}

	c1 := findBoundary(data, n/3)
	c2 := findBoundary(data, (2*n)/3)
	if c2 < c1 {
		c2 = c1
	}

	return [][]byte{
		bytes.TrimSpace(data[:c1]),
		bytes.TrimSpace(data[c1:c2]),
		bytes.TrimSpace(data[c2:]),
	}
}

func findBoundary(data []byte, start int) int {
	if start < 0 {
		start = 0
	}
	if start >= len(data) {
		return len(data)
	}
	for i := start; i < len(data); i++ {
		if unicode.IsSpace(rune(data[i])) {
			return i + 1
		}
	}
	return len(data)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func strPtr(s string) *string { return &s }
