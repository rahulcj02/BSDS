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

type mapResponse struct {
	RunID      string `json:"run_id"`
	Output     string `json:"output"`
	DurationMS int64  `json:"duration_ms"`
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

	mux.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
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
			outPrefix = "maps"
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

		counts := wordCount(string(data))
		outBytes, err := json.MarshalIndent(counts, "", "  ")
		if err != nil {
			http.Error(w, fmt.Sprintf("json marshal failed: %v", err), http.StatusInternalServerError)
			return
		}

		runID := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
		outKey := fmt.Sprintf("%s/%s/result.json", outPrefix, runID)

		_, err = s3c.PutObject(reqCtx, &s3.PutObjectInput{
			Bucket:      &outBucket,
			Key:         &outKey,
			Body:        bytes.NewReader(outBytes),
			ContentType: strPtr("application/json"),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("PutObject failed: %v", err), http.StatusInternalServerError)
			return
		}

		resp := mapResponse{
			RunID:      runID,
			Output:     fmt.Sprintf("s3://%s/%s", outBucket, outKey),
			DurationMS: time.Since(start).Milliseconds(),
		}
		writeJSON(w, resp)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("mapper listening on :%s", port)
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

func wordCount(s string) map[string]int {
	out := map[string]int{}
	var b strings.Builder

	flush := func() {
		if b.Len() == 0 {
			return
		}
		w := b.String()
		b.Reset()
		out[w]++
	}

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '\'' {
			b.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()

	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func strPtr(s string) *string { return &s }
