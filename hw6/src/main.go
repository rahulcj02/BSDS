package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

type SearchResponse struct {
	Products        []Product `json:"products"`
	TotalFound      int       `json:"total_found"`
	CheckedProducts int       `json:"checked_products"`
	SearchTime      string    `json:"search_time"`
}

type errorResponse struct {
	Error string `json:"error"`
}

const (
	totalProducts = 100000
	scanWindow    = 100
	maxResults    = 20
)

var (
	products      sync.Map
	productIDs    []int
	searchCursor  atomic.Uint64
	initializeMux sync.Mutex
	initialized   bool
)

func ensureProducts() {
	if initialized {
		return
	}
	initializeMux.Lock()
	defer initializeMux.Unlock()
	if initialized {
		return
	}
	brands := []string{"Alpha", "Beta", "Gamma", "Delta", "Omega", "Nova", "Vertex", "Pulse", "Nimbus", "Atlas"}
	categories := []string{"Electronics", "Books", "Home", "Sports", "Beauty", "Grocery", "Toys", "Automotive"}
	descriptors := []string{"durable", "compact", "premium", "smart", "eco", "portable", "advanced", "lightweight"}
	productIDs = make([]int, totalProducts)
	for i := 0; i < totalProducts; i++ {
		id := i + 1
		brand := brands[i%len(brands)]
		category := categories[i%len(categories)]
		descriptor := descriptors[i%len(descriptors)]
		product := Product{
			ID:          id,
			Name:        "Product " + brand + " " + strconv.Itoa(id),
			Category:    category,
			Description: fmt.Sprintf("%s %s item %d", descriptor, category, id),
			Brand:       brand,
		}
		products.Store(id, product)
		productIDs[i] = id
	}
	initialized = true
}

func main() {
	ensureProducts()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/products/search", searchHandler)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"products_loaded": totalProducts,
	})
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method_not_allowed"})
		return
	}
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	if term == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "query parameter q is required"})
		return
	}
	response := searchProducts(term)
	writeJSON(w, http.StatusOK, response)
}

func searchProducts(term string) SearchResponse {
	ensureProducts()
	start := time.Now()
	needle := strings.ToLower(term)
	results := make([]Product, 0, maxResults)
	totalFound := 0
	startIndex := int(searchCursor.Add(scanWindow)-scanWindow) % totalProducts
	for checked := 0; checked < scanWindow; checked++ {
		idx := (startIndex + checked) % totalProducts
		id := productIDs[idx]
		raw, ok := products.Load(id)
		if !ok {
			continue
		}
		p := raw.(Product)
		if strings.Contains(strings.ToLower(p.Name), needle) || strings.Contains(strings.ToLower(p.Category), needle) {
			totalFound++
			if len(results) < maxResults {
				results = append(results, p)
			}
		}
	}
	return SearchResponse{
		Products:        results,
		TotalFound:      totalFound,
		CheckedProducts: scanWindow,
		SearchTime:      time.Since(start).String(),
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
