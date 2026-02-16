// src/main.go
package main

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type Product struct {
	ProductID    int    `json:"product_id"`
	SKU          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryID   int    `json:"category_id"`
	Weight       int    `json:"weight"`
	SomeOtherID  int    `json:"some_other_id"`
}

type ErrorResp struct {
	Error   string      `json:"error"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type Store struct {
	mu       sync.RWMutex
	products map[int]Product
}

func main() {
	store := &Store{
		products: map[int]Product{
			1: {
				ProductID:    1,
				SKU:          "SEED-001",
				Manufacturer: "Seed Manufacturer",
				CategoryID:   10,
				Weight:       0,
				SomeOtherID:  100,
			},
			2: {
				ProductID:    2,
				SKU:          "SEED-002",
				Manufacturer: "Seed Manufacturer",
				CategoryID:   20,
				Weight:       5,
				SomeOtherID:  200,
			},
		},
	}

	router := gin.Default()
	router.GET("/products/:productId", store.getProduct)
	router.POST("/products/:productId/details", store.postProductDetails)
	router.Run(":8080")
}

func (s *Store) getProduct(c *gin.Context) {
	productID, ok := parseProductID(c.Param("productId"))
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResp{
			Error:   "bad_request",
			Message: "productId must be an integer >= 1",
		})
		return
	}

	s.mu.RLock()
	p, exists := s.products[productID]
	s.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, ErrorResp{
			Error:   "not_found",
			Message: "product not found",
		})
		return
	}

	c.JSON(http.StatusOK, p)
}

func (s *Store) postProductDetails(c *gin.Context) {
	productID, ok := parseProductID(c.Param("productId"))
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResp{
			Error:   "bad_request",
			Message: "productId must be an integer >= 1",
		})
		return
	}

	s.mu.RLock()
	_, exists := s.products[productID]
	s.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, ErrorResp{
			Error:   "not_found",
			Message: "product not found",
		})
		return
	}

	var body Product
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResp{
			Error:   "bad_request",
			Message: "invalid JSON body",
			Details: err.Error(),
		})
		return
	}

	if errs := validateProduct(body, productID); len(errs) > 0 {
		c.JSON(http.StatusBadRequest, ErrorResp{
			Error:   "bad_request",
			Message: "validation failed",
			Details: errs,
		})
		return
	}

	s.mu.Lock()
	s.products[productID] = body
	s.mu.Unlock()

	c.Status(http.StatusNoContent)
}

func parseProductID(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func validateProduct(p Product, pathProductID int) []string {
	var errs []string

	if p.ProductID < 1 {
		errs = append(errs, "product_id must be >= 1")
	}
	if p.ProductID != pathProductID {
		errs = append(errs, "product_id must match productId in path")
	}

	if l := len(p.SKU); l < 1 || l > 100 {
		errs = append(errs, "sku length must be between 1 and 100")
	}
	if strings.TrimSpace(p.SKU) == "" {
		errs = append(errs, "sku must not be blank")
	}

	if l := len(p.Manufacturer); l < 1 || l > 200 {
		errs = append(errs, "manufacturer length must be between 1 and 200")
	}
	if strings.TrimSpace(p.Manufacturer) == "" {
		errs = append(errs, "manufacturer must not be blank")
	}

	if p.CategoryID < 1 {
		errs = append(errs, "category_id must be >= 1")
	}
	if p.Weight < 0 {
		errs = append(errs, "weight must be >= 0")
	}
	if p.SomeOtherID < 1 {
		errs = append(errs, "some_other_id must be >= 1")
	}

	return errs
}
