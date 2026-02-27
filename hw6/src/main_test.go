package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestMux() *http.ServeMux {
	ensureProducts()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/products/search", searchHandler)
	return mux
}

func TestHealth(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", res.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok got %v", body["status"])
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest(http.MethodGet, "/products/search", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", res.Code)
	}
}

func TestSearchChecksExactlyHundred(t *testing.T) {
	result := searchProducts("electronics")
	if result.CheckedProducts != 100 {
		t.Fatalf("expected 100 checked got %d", result.CheckedProducts)
	}
	if len(result.Products) > 20 {
		t.Fatalf("expected max 20 results got %d", len(result.Products))
	}
}

func TestSearchCaseInsensitiveOnCategory(t *testing.T) {
	result := searchProducts("ELECTRONICS")
	if result.TotalFound < 1 {
		t.Fatalf("expected at least one match got %d", result.TotalFound)
	}
}

func TestSearchEndpoint(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest(http.MethodGet, "/products/search?q=books", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", res.Code)
	}
	var body SearchResponse
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.CheckedProducts != 100 {
		t.Fatalf("expected 100 checked got %d", body.CheckedProducts)
	}
}
