package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/mux"
)

type Product struct {
	ProductID    int    `json:"product_id"`
	SKU          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryID   int    `json:"category_id"`
	Weight       int    `json:"weight"`
	SomeOtherID  int    `json:"some_other_id"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

var (
	products = make(map[int]Product)
	mu       sync.RWMutex
)

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/v1/products/{productId}", getProduct).Methods("GET")
	r.HandleFunc("/v1/products/{productId}/details", addProductDetails).Methods("POST")

	fmt.Println("Server starting on port 8080...")
	http.ListenAndServe(":8080", r)
}

func getProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["productId"])

	if err != nil || id < 1 {
		sendError(w, "INVALID_INPUT", "Product ID must be a positive integer", http.StatusBadRequest)
		return
	}

	mu.RLock()
	product, exists := products[id]
	mu.RUnlock()

	if !exists {
		sendError(w, "NOT_FOUND", "Product not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(product)
}

func addProductDetails(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["productId"])

	if err != nil || id < 1 {
		sendError(w, "INVALID_INPUT", "Product ID must be a positive integer", http.StatusBadRequest)
		return
	}

	var p Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		sendError(w, "INVALID_INPUT", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if p.SKU == "" || p.Manufacturer == "" || p.ProductID != id {
		sendError(w, "INVALID_INPUT", "Missing required fields or ID mismatch", http.StatusBadRequest)
		return
	}

	mu.Lock()
	products[id] = p
	mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func sendError(w http.ResponseWriter, code string, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: code, Message: message})
}
