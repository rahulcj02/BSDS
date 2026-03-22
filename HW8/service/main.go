package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	defaultPort = "8080"
)

var (
	errNotFound = errors.New("not found")
)

type CartItem struct {
	ProductID int64 `json:"product_id"`
	Quantity  int64 `json:"quantity"`
}

type Cart struct {
	ShoppingCartID int64      `json:"shopping_cart_id"`
	CustomerID     int64      `json:"customer_id"`
	Items          []CartItem `json:"items"`
	CreatedAt      time.Time  `json:"created_at"`
}

type Store interface {
	CreateCart(ctx context.Context, customerID int64) (int64, error)
	AddItem(ctx context.Context, cartID, productID, quantity int64) error
	GetCart(ctx context.Context, cartID int64) (*Cart, error)
	Close() error
}

type server struct {
	store Store
}

func main() {
	rand.Seed(time.Now().UnixNano())

	store, err := initStore()
	if err != nil {
		log.Fatalf("init store: %v", err)
	}
	defer store.Close()

	s := &server{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/shopping-carts", s.handleCarts)
	mux.HandleFunc("/shopping-carts/", s.handleCartByID)

	port := getenv("PORT", defaultPort)
	addr := ":" + port
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, logRequest(mux)); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func initStore() (Store, error) {
	backend := strings.ToLower(getenv("DB_BACKEND", "mysql"))
	switch backend {
	case "mysql":
		return initMySQL()
	case "dynamodb":
		return initDynamo()
	default:
		return nil, fmt.Errorf("unsupported DB_BACKEND=%s", backend)
	}
}

func initMySQL() (Store, error) {
	host := getenv("DB_HOST", "")
	user := getenv("DB_USER", "")
	pass := getenv("DB_PASS", "")
	dbname := getenv("DB_NAME", "")
	port := getenv("DB_PORT", "3306")
	if host == "" || user == "" || pass == "" || dbname == "" {
		return nil, errors.New("DB_HOST, DB_USER, DB_PASS, DB_NAME must be set for mysql")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true",
		user, pass, host, port, dbname)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	maxOpen := getenvInt("DB_MAX_OPEN", 20)
	maxIdle := getenvInt("DB_MAX_IDLE", 10)
	maxLife := time.Duration(getenvInt("DB_MAX_LIFETIME_SEC", 300)) * time.Second
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLife)

	if err := db.Ping(); err != nil {
		return nil, err
	}
	if err := ensureSchema(db); err != nil {
		return nil, err
	}

	return &mysqlStore{db: db}, nil
}

func ensureSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS carts (
	cart_id BIGINT AUTO_INCREMENT PRIMARY KEY,
	customer_id BIGINT NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	INDEX idx_carts_customer_id (customer_id)
);
CREATE TABLE IF NOT EXISTS cart_items (
	cart_id BIGINT NOT NULL,
	product_id BIGINT NOT NULL,
	quantity BIGINT NOT NULL,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	PRIMARY KEY (cart_id, product_id),
	CONSTRAINT fk_cart_items_cart_id FOREIGN KEY (cart_id) REFERENCES carts(cart_id) ON DELETE CASCADE
);`
	_, err := db.Exec(schema)
	return err
}

type mysqlStore struct {
	db *sql.DB
}

func (m *mysqlStore) Close() error { return m.db.Close() }

func (m *mysqlStore) CreateCart(ctx context.Context, customerID int64) (int64, error) {
	res, err := m.db.ExecContext(ctx, "INSERT INTO carts (customer_id) VALUES (?)", customerID)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (m *mysqlStore) AddItem(ctx context.Context, cartID, productID, quantity int64) error {
	var exists bool
	if err := m.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM carts WHERE cart_id=?)", cartID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errNotFound
	}
	_, err := m.db.ExecContext(ctx, `
INSERT INTO cart_items (cart_id, product_id, quantity)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE quantity=VALUES(quantity)`,
		cartID, productID, quantity)
	return err
}

func (m *mysqlStore) GetCart(ctx context.Context, cartID int64) (*Cart, error) {
	var cart Cart
	row := m.db.QueryRowContext(ctx, "SELECT cart_id, customer_id, created_at FROM carts WHERE cart_id=?", cartID)
	if err := row.Scan(&cart.ShoppingCartID, &cart.CustomerID, &cart.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errNotFound
		}
		return nil, err
	}
	items := []CartItem{}
	rows, err := m.db.QueryContext(ctx, "SELECT product_id, quantity FROM cart_items WHERE cart_id=?", cartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it CartItem
		if err := rows.Scan(&it.ProductID, &it.Quantity); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	cart.Items = items
	return &cart, nil
}

type dynamoStore struct {
	client *dynamodb.Client
	table  string
}

func initDynamo() (Store, error) {
	table := getenv("DDB_TABLE", "")
	if table == "" {
		return nil, errors.New("DDB_TABLE must be set for dynamodb")
	}
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}
	return &dynamoStore{
		client: dynamodb.NewFromConfig(cfg),
		table:  table,
	}, nil
}

func (d *dynamoStore) Close() error { return nil }

func (d *dynamoStore) CreateCart(ctx context.Context, customerID int64) (int64, error) {
	cartID := generateCartID()
	item := map[string]types.AttributeValue{
		"cart_id":     &types.AttributeValueMemberS{Value: strconv.FormatInt(cartID, 10)},
		"customer_id": &types.AttributeValueMemberN{Value: strconv.FormatInt(customerID, 10)},
		"created_at":  &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339Nano)},
		"items":       &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}},
	}
	_, err := d.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(d.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(cart_id)"),
	})
	if err != nil {
		return 0, err
	}
	return cartID, nil
}

func (d *dynamoStore) AddItem(ctx context.Context, cartID, productID, quantity int64) error {
	// Read current cart to avoid UpdateItem permissions issues and to merge items safely.
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberS{Value: strconv.FormatInt(cartID, 10)},
		},
		ConsistentRead: aws.Bool(false),
	})
	if err != nil {
		return err
	}
	if out.Item == nil {
		return errNotFound
	}

	items := map[string]types.AttributeValue{}
	if m, ok := out.Item["items"].(*types.AttributeValueMemberM); ok {
		for k, v := range m.Value {
			items[k] = v
		}
	}
	items[strconv.FormatInt(productID, 10)] = &types.AttributeValueMemberN{Value: strconv.FormatInt(quantity, 10)}

	out.Item["items"] = &types.AttributeValueMemberM{Value: items}
	_, err = d.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(d.table),
		Item:                out.Item,
		ConditionExpression: aws.String("attribute_exists(cart_id)"),
	})
	if err != nil {
		var cce *types.ConditionalCheckFailedException
		if errors.As(err, &cce) {
			return errNotFound
		}
		return err
	}
	return nil
}

func (d *dynamoStore) GetCart(ctx context.Context, cartID int64) (*Cart, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberS{Value: strconv.FormatInt(cartID, 10)},
		},
		ConsistentRead: aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, errNotFound
	}
	cart := &Cart{}
	cartIDStr := getString(out.Item["cart_id"])
	cartIDParsed, _ := strconv.ParseInt(cartIDStr, 10, 64)
	cart.ShoppingCartID = cartIDParsed
	cart.CustomerID = getInt(out.Item["customer_id"])
	if ts := getString(out.Item["created_at"]); ts != "" {
		cart.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	}
	items := []CartItem{}
	if m, ok := out.Item["items"].(*types.AttributeValueMemberM); ok {
		for k, v := range m.Value {
			pid, _ := strconv.ParseInt(k, 10, 64)
			qty := getInt(v)
			items = append(items, CartItem{ProductID: pid, Quantity: qty})
		}
	}
	cart.Items = items
	return cart, nil
}

func generateCartID() int64 {
	// Generate a reasonably unique, positive int64 fitting in int32 range for API compatibility.
	return (time.Now().UnixNano()%900000000 + 100000000) + int64(rand.Intn(1000))
}

func getString(av types.AttributeValue) string {
	if s, ok := av.(*types.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}

func getInt(av types.AttributeValue) int64 {
	if n, ok := av.(*types.AttributeValueMemberN); ok {
		val, _ := strconv.ParseInt(n.Value, 10, 64)
		return val
	}
	return 0
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *server) handleCarts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		CustomerID int64 `json:"customer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.CustomerID <= 0 {
		http.Error(w, "invalid customer_id", http.StatusBadRequest)
		return
	}
	id, err := s.store.CreateCart(r.Context(), payload.CustomerID)
	if err != nil {
		http.Error(w, "failed to create cart", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]int64{"shopping_cart_id": id})
}

func (s *server) handleCartByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/shopping-carts/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	cartID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || cartID <= 0 {
		http.Error(w, "invalid cart id", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		s.handleGetCart(w, r, cartID)
		return
	}
	if len(parts) == 2 && parts[1] == "items" && r.Method == http.MethodPost {
		s.handleAddItems(w, r, cartID)
		return
	}

	http.NotFound(w, r)
}

func (s *server) handleAddItems(w http.ResponseWriter, r *http.Request, cartID int64) {
	var payload struct {
		ProductID int64 `json:"product_id"`
		Quantity  int64 `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.ProductID <= 0 || payload.Quantity <= 0 {
		http.Error(w, "invalid product_id or quantity", http.StatusBadRequest)
		return
	}
	if err := s.store.AddItem(r.Context(), cartID, payload.ProductID, payload.Quantity); err != nil {
		if errors.Is(err, errNotFound) {
			http.Error(w, "cart not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to add item", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleGetCart(w http.ResponseWriter, r *http.Request, cartID int64) {
	cart, err := s.store.GetCart(r.Context(), cartID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.Error(w, "cart not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to get cart", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cart)
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func getenv(key, def string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	return val
}

func getenvInt(key string, def int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return i
}
