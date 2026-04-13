package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

var (
	db       *sql.DB
	s3Client *s3.Client
	bucket   string
	region   string
	baseURL  string // public base URL for photo downloads
)

func main() {
	var err error

	// Config from environment
	dbURL := os.Getenv("DATABASE_URL") // e.g. postgres://user:pass@host:5432/albumstore?sslmode=disable
	bucket = os.Getenv("S3_BUCKET")
	region = os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}
	baseURL = os.Getenv("BASE_URL") // e.g. http://your-ec2-ip:8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Database
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Connected to database")

	// Run migrations
	if err := migrate(); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// S3
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	s3Client = s3.NewFromConfig(cfg)

	// Start background workers
	for i := 0; i < 5; i++ {
		go photoWorker(i)
	}

	// Routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/albums", handleAlbums)
	mux.HandleFunc("/albums/", handleAlbumsRouter)

	log.Printf("Starting server on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS albums (
			album_id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			owner TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS photos (
			photo_id TEXT PRIMARY KEY,
			album_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'processing',
			url TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS album_seq (
			album_id TEXT PRIMARY KEY,
			next_seq INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_photos_album_id ON photos(album_id)`,
		`CREATE INDEX IF NOT EXISTS idx_photos_status ON photos(status)`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("migration failed: %w — query: %s", err, q)
		}
	}
	log.Println("Migrations complete")
	return nil
}

// ─── HANDLERS ───────────────────────────────────────────────────────────────

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func handleAlbums(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	listAlbums(w, r)
}

func handleAlbumsRouter(w http.ResponseWriter, r *http.Request) {
	// Parse: /albums/{album_id} or /albums/{album_id}/photos or /albums/{album_id}/photos/{photo_id}
	path := strings.TrimPrefix(r.URL.Path, "/albums/")
	parts := strings.Split(path, "/")

	switch {
	case len(parts) == 1 && parts[0] != "":
		// /albums/{album_id}
		albumID := parts[0]
		switch r.Method {
		case http.MethodPut:
			putAlbum(w, r, albumID)
		case http.MethodGet:
			getAlbum(w, r, albumID)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}

	case len(parts) == 2 && parts[1] == "photos":
		// /albums/{album_id}/photos
		albumID := parts[0]
		if r.Method == http.MethodPost {
			uploadPhoto(w, r, albumID)
		} else {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}

	case len(parts) == 3 && parts[1] == "photos" && parts[2] != "":
		// /albums/{album_id}/photos/{photo_id}
		albumID := parts[0]
		photoID := parts[2]
		switch r.Method {
		case http.MethodGet:
			getPhoto(w, r, albumID, photoID)
		case http.MethodDelete:
			deletePhoto(w, r, albumID, photoID)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}

	default:
		http.NotFound(w, r)
	}
}

// ─── ALBUM CRUD ─────────────────────────────────────────────────────────────

func putAlbum(w http.ResponseWriter, r *http.Request, albumID string) {
	var req struct {
		AlbumID     string `json:"album_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Owner       string `json:"owner"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}

	// Use album_id from path
	if req.AlbumID == "" {
		req.AlbumID = albumID
	}

	// Upsert
	result, err := db.Exec(`
		INSERT INTO albums (album_id, title, description, owner)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (album_id) DO UPDATE
		SET title = EXCLUDED.title, description = EXCLUDED.description, owner = EXCLUDED.owner
	`, req.AlbumID, req.Title, req.Description, req.Owner)
	if err != nil {
		log.Printf("putAlbum error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	status := http.StatusOK
	// PostgreSQL ON CONFLICT DO UPDATE always returns 1 row affected, so we check differently
	// We'll use a different approach: try insert, if conflict then update
	// Actually, let's just always return 200 for simplicity — the spec says 200 or 201
	_ = rowsAffected

	resp := map[string]string{
		"album_id":    req.AlbumID,
		"title":       req.Title,
		"description": req.Description,
		"owner":       req.Owner,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func getAlbum(w http.ResponseWriter, r *http.Request, albumID string) {
	var a struct {
		AlbumID     string `json:"album_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Owner       string `json:"owner"`
	}
	err := db.QueryRow(`SELECT album_id, title, description, owner FROM albums WHERE album_id = $1`, albumID).
		Scan(&a.AlbumID, &a.Title, &a.Description, &a.Owner)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		log.Printf("getAlbum error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func listAlbums(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT album_id, title, description, owner FROM albums`)
	if err != nil {
		log.Printf("listAlbums error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}
	defer rows.Close()

	albums := make([]map[string]string, 0)
	for rows.Next() {
		var albumID, title, desc, owner string
		if err := rows.Scan(&albumID, &title, &desc, &owner); err != nil {
			continue
		}
		albums = append(albums, map[string]string{
			"album_id":    albumID,
			"title":       title,
			"description": desc,
			"owner":       owner,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(albums)
}

// ─── PHOTO UPLOAD (ASYNC) ───────────────────────────────────────────────────

func uploadPhoto(w http.ResponseWriter, r *http.Request, albumID string) {
	// Parse multipart — limit to 210MB
	if err := r.ParseMultipartForm(210 << 20); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid multipart form"})
		return
	}

	file, _, err := r.FormFile("photo")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing photo field"})
		return
	}
	defer file.Close()

	// Read file into memory for background processing
	data, err := io.ReadAll(file)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read photo"})
		return
	}

	photoID := uuid.New().String()

	// Assign seq atomically using a transaction
	var seq int
	tx, err := db.Begin()
	if err != nil {
		log.Printf("uploadPhoto tx begin error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}

	// Upsert album_seq and get next seq atomically
	err = tx.QueryRow(`
		INSERT INTO album_seq (album_id, next_seq)
		VALUES ($1, 2)
		ON CONFLICT (album_id) DO UPDATE SET next_seq = album_seq.next_seq + 1
		RETURNING next_seq - 1
	`, albumID).Scan(&seq)
	if err != nil {
		tx.Rollback()
		log.Printf("uploadPhoto seq error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}

	// Insert photo record with status=processing
	_, err = tx.Exec(`
		INSERT INTO photos (photo_id, album_id, seq, status)
		VALUES ($1, $2, $3, 'processing')
	`, photoID, albumID, seq)
	if err != nil {
		tx.Rollback()
		log.Printf("uploadPhoto insert error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("uploadPhoto commit error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}

	// Enqueue for background processing
	photoQueue <- photoJob{
		PhotoID: photoID,
		AlbumID: albumID,
		Data:    data,
	}

	// Return 202 immediately
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"photo_id": photoID,
		"seq":      seq,
		"status":   "processing",
	})
}

// ─── PHOTO STATUS & DELETE ──────────────────────────────────────────────────

func getPhoto(w http.ResponseWriter, r *http.Request, albumID string, photoID string) {
	var p struct {
		PhotoID string         `json:"photo_id"`
		AlbumID string         `json:"album_id"`
		Seq     int            `json:"seq"`
		Status  string         `json:"status"`
		URL     sql.NullString `json:"-"`
	}
	err := db.QueryRow(`
		SELECT photo_id, album_id, seq, status, url
		FROM photos WHERE photo_id = $1 AND album_id = $2
	`, photoID, albumID).Scan(&p.PhotoID, &p.AlbumID, &p.Seq, &p.Status, &p.URL)

	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		log.Printf("getPhoto error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}

	resp := map[string]interface{}{
		"photo_id": p.PhotoID,
		"album_id": p.AlbumID,
		"seq":      p.Seq,
		"status":   p.Status,
	}
	if p.Status == "completed" && p.URL.Valid {
		resp["url"] = p.URL.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func deletePhoto(w http.ResponseWriter, r *http.Request, albumID string, photoID string) {
	// Get URL before deleting
	var url sql.NullString
	err := db.QueryRow(`SELECT url FROM photos WHERE photo_id = $1 AND album_id = $2`, photoID, albumID).Scan(&url)
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		log.Printf("deletePhoto select error: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Delete from S3 if URL exists
	if url.Valid && url.String != "" {
		s3Key := extractS3Key(url.String)
		if s3Key != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			_, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(s3Key),
			})
			if err != nil {
				log.Printf("deletePhoto S3 delete error: %v", err)
				// Continue anyway — delete the DB record
			}
		}
	}

	// Delete from DB
	_, err = db.Exec(`DELETE FROM photos WHERE photo_id = $1 AND album_id = $2`, photoID, albumID)
	if err != nil {
		log.Printf("deletePhoto DB error: %v", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

func extractS3Key(urlStr string) string {
	// URL format: https://{bucket}.s3.{region}.amazonaws.com/{key}
	// or https://s3.{region}.amazonaws.com/{bucket}/{key}
	parts := strings.SplitN(urlStr, bucket+".s3.", 2)
	if len(parts) == 2 {
		// Find the key after the host
		idx := strings.Index(parts[1], "/")
		if idx >= 0 {
			return parts[1][idx+1:]
		}
	}
	// Try alternate format
	parts = strings.SplitN(urlStr, bucket+"/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	// Fallback: extract path from URL
	idx := strings.Index(urlStr, ".amazonaws.com/")
	if idx >= 0 {
		key := urlStr[idx+len(".amazonaws.com/"):]
		// If the path starts with bucket name, strip it
		if strings.HasPrefix(key, bucket+"/") {
			key = strings.TrimPrefix(key, bucket+"/")
		}
		return key
	}
	return ""
}

// ─── BACKGROUND WORKER ─────────────────────────────────────────────────────

type photoJob struct {
	PhotoID string
	AlbumID string
	Data    []byte
}

var photoQueue = make(chan photoJob, 1000)

func photoWorker(id int) {
	log.Printf("Photo worker %d started", id)
	for job := range photoQueue {
		processPhoto(job)
	}
}

func processPhoto(job photoJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	key := fmt.Sprintf("albums/%s/photos/%s.jpg", job.AlbumID, job.PhotoID)

	// Upload to S3
	_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        strings.NewReader(string(job.Data)),
		ContentType: aws.String("image/jpeg"),
	})
	if err != nil {
		log.Printf("processPhoto S3 upload error for %s: %v", job.PhotoID, err)
		db.Exec(`UPDATE photos SET status = 'failed' WHERE photo_id = $1`, job.PhotoID)
		return
	}

	// Build public URL
	photoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, region, key)

	// Update status to completed
	_, err = db.Exec(`UPDATE photos SET status = 'completed', url = $1 WHERE photo_id = $2`, photoURL, job.PhotoID)
	if err != nil {
		log.Printf("processPhoto DB update error for %s: %v", job.PhotoID, err)
		return
	}
	log.Printf("Photo %s processed successfully", job.PhotoID)
}
