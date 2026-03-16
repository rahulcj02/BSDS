package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
)

const (
	defaultPort       = "8080"
	defaultWorkerCount = 1
)

type Item struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"`
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

type PaymentLimiter struct {
	sem   chan struct{}
	delay time.Duration
}

func NewPaymentLimiter(delay time.Duration, capacity int) *PaymentLimiter {
	if capacity < 1 {
		capacity = 1
	}
	return &PaymentLimiter{
		sem:   make(chan struct{}, capacity),
		delay: delay,
	}
}

func (p *PaymentLimiter) Process(ctx context.Context) error {
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.sem }()

	timer := time.NewTimer(p.delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type app struct {
	payment  *PaymentLimiter
	sns      *sns.Client
	snsTopic string
	logger   *log.Logger
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	mode := getenv("MODE", "api")
	region := getenv("AWS_REGION", "")

	loadOpts := []func(*config.LoadOptions) error{}
	if region != "" {
		loadOpts = append(loadOpts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		logger.Fatalf("failed to load AWS config: %v", err)
	}

	switch mode {
	case "api":
		runAPI(cfg, logger)
	case "processor":
		runProcessor(cfg, logger)
	default:
		logger.Fatalf("unknown MODE: %s", mode)
	}
}

func runAPI(cfg aws.Config, logger *log.Logger) {
	payment := NewPaymentLimiter(3*time.Second, 1)
	snsClient := sns.NewFromConfig(cfg)
	app := &app{
		payment:  payment,
		sns:      snsClient,
		snsTopic: getenv("SNS_TOPIC_ARN", ""),
		logger:   logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/orders/sync", app.handleSync)
	mux.HandleFunc("/orders/async", app.handleAsync)

	port := getenv("PORT", defaultPort)
	logger.Printf("API listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logger.Fatalf("server stopped: %v", err)
	}
}

func (a *app) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	order, err := decodeOrder(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	order.Status = "processing"

	if err := a.payment.Process(r.Context()); err != nil {
		http.Error(w, "payment processing failed", http.StatusRequestTimeout)
		return
	}
	order.Status = "completed"

	writeJSON(w, http.StatusOK, order)
}

func (a *app) handleAsync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.snsTopic == "" {
		http.Error(w, "SNS topic not configured", http.StatusInternalServerError)
		return
	}

	order, err := decodeOrder(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	order.Status = "pending"

	payload, err := json.Marshal(order)
	if err != nil {
		http.Error(w, "failed to encode order", http.StatusInternalServerError)
		return
	}

	_, err = a.sns.Publish(r.Context(), &sns.PublishInput{
		TopicArn: aws.String(a.snsTopic),
		Message:  aws.String(string(payload)),
	})
	if err != nil {
		a.logger.Printf("sns publish failed: %v", err)
		http.Error(w, "failed to publish order", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, order)
}

func runProcessor(cfg aws.Config, logger *log.Logger) {
	queueURL := getenv("SQS_QUEUE_URL", "")
	if queueURL == "" {
		logger.Fatalf("SQS_QUEUE_URL is required for processor mode")
	}

	workerCount := getenvInt("WORKER_COUNT", defaultWorkerCount)
	if workerCount < 1 {
		workerCount = defaultWorkerCount
	}

	logger.Printf("processor starting: workers=%d", workerCount)
	client := sqs.NewFromConfig(cfg)
	payment := NewPaymentLimiter(3*time.Second, workerCount)
	sem := make(chan struct{}, workerCount)

	for {
		resp, err := client.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
		})
		if err != nil {
			logger.Printf("receive error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, msg := range resp.Messages {
			sem <- struct{}{}
			go func(m types.Message) {
				defer func() { <-sem }()
				if err := handleMessage(client, payment, queueURL, m, logger); err != nil {
					logger.Printf("message failed: %v", err)
				}
			}(msg)
		}
	}
}

func handleMessage(client *sqs.Client, payment *PaymentLimiter, queueURL string, msg types.Message, logger *log.Logger) error {
	body := aws.ToString(msg.Body)
	if body == "" {
		return errors.New("empty message body")
	}

	order, err := parseOrderFromMessage(body)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := payment.Process(ctx); err != nil {
		return err
	}

	_, err = client.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		logger.Printf("delete failed: %v", err)
		return err
	}

	logger.Printf("processed order %s", order.OrderID)
	return nil
}

type snsEnvelope struct {
	Message string `json:"Message"`
}

func parseOrderFromMessage(body string) (Order, error) {
	var order Order
	if err := json.Unmarshal([]byte(body), &order); err == nil && order.OrderID != "" {
		return order, nil
	}

	var env snsEnvelope
	if err := json.Unmarshal([]byte(body), &env); err == nil && env.Message != "" {
		if err := json.Unmarshal([]byte(env.Message), &order); err != nil {
			return Order{}, err
		}
		return order, nil
	}

	return Order{}, errors.New("unable to parse order message")
}

func decodeOrder(r *http.Request) (Order, error) {
	defer r.Body.Close()

	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		return Order{}, errors.New("invalid JSON body")
	}
	if order.CustomerID == 0 {
		return Order{}, errors.New("customer_id is required")
	}
	if order.OrderID == "" {
		order.OrderID = uuid.New().String()
	}
	if order.CreatedAt.IsZero() {
		order.CreatedAt = time.Now().UTC()
	}
	if order.Items == nil {
		order.Items = []Item{}
	}
	return order, nil
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
