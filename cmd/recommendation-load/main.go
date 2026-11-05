// recommendation-load sends synthetic authenticated traffic to a staging deployment.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/example/e-commerce-be/internal/modules/recommendation"
	"github.com/google/uuid"
)

type task struct {
	token    string
	behavior bool
	events   int
}
type metrics struct {
	mu         sync.Mutex
	latencies  []time.Duration
	ok, failed int64
}

func (m *metrics) record(duration time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if success {
		m.ok++
		m.latencies = append(m.latencies, duration)
	} else {
		m.failed++
	}
}
func (m *metrics) print(label string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sort.Slice(m.latencies, func(i, j int) bool { return m.latencies[i] < m.latencies[j] })
	var p95 time.Duration
	if len(m.latencies) > 0 {
		p95 = m.latencies[(len(m.latencies)*95-1)/100]
	}
	fmt.Printf("%s ok=%d failed=%d p95=%s\n", label, m.ok, m.failed, p95)
}

func main() {
	base := flag.String("url", "http://localhost:8080", "API base URL")
	tokenFile := flag.String("tokens", "", "path to newline-separated staging access JWTs")
	productRaw := flag.String("product", "", "published product UUID")
	duration := flag.Duration("duration", 30*time.Minute, "test duration")
	eventRate := flag.Int("events", 1000, "behavior events per second")
	recRate := flag.Int("recommendations", 200, "recommendation requests per second")
	workers := flag.Int("workers", 256, "maximum concurrent requests")
	query := flag.String("query", "giay", "submitted search query")
	flag.Parse()
	product, err := uuid.Parse(*productRaw)
	if err != nil || product == uuid.Nil || *tokenFile == "" || *eventRate < 0 || *recRate < 0 || *workers < 1 || *duration <= 0 {
		fmt.Fprintln(os.Stderr, "invalid load test parameters")
		os.Exit(2)
	}
	content, err := os.ReadFile(*tokenFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot read token file")
		os.Exit(2)
	}
	tokens := strings.Fields(string(content))
	if len(tokens) == 0 || len(tokens) < (*eventRate+1)/2 {
		fmt.Fprintf(os.Stderr, "need at least %d distinct user tokens to remain below 120 events/min/user\n", (*eventRate+1)/2)
		os.Exit(2)
	}
	deadline := time.Now().Add(*duration + time.Minute)
	for _, token := range tokens {
		if !accessTokenLasts(token, deadline) {
			fmt.Fprintln(os.Stderr, "all staging access tokens must remain valid through the test duration")
			os.Exit(2)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	jobs := make(chan task, 4096)
	client := &http.Client{Timeout: 4 * time.Second}
	var sent atomic.Int64
	var behavior, reads metrics
	var group sync.WaitGroup
	for i := 0; i < *workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for job := range jobs {
				start := time.Now()
				var ok bool
				if job.behavior {
					ok = sendEvents(ctx, client, *base, job.token, product, *query, job.events, &sent)
				} else {
					ok = readRecommendations(ctx, client, *base, job.token, product)
				}
				if job.behavior {
					behavior.record(time.Since(start), ok)
				} else {
					reads.record(time.Since(start), ok)
				}
			}
		}()
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	var eventBudget, readBudget float64
	eventIndex, readIndex := 0, 0
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-tick.C:
			eventBudget += float64(*eventRate) / 100
			readBudget += float64(*recRate) / 100
			for eventBudget >= 1 {
				count := min(20, int(eventBudget))
				eventBudget -= float64(count)
				if !queue(ctx, jobs, task{token: tokens[eventIndex%len(tokens)], behavior: true, events: count}) {
					break loop
				}
				eventIndex++
			}
			for readBudget >= 1 {
				readBudget--
				if !queue(ctx, jobs, task{token: tokens[readIndex%len(tokens)]}) {
					break loop
				}
				readIndex++
			}
		}
	}
	close(jobs)
	group.Wait()
	behavior.print("behavior requests")
	reads.print("recommendation requests")
	fmt.Printf("behavior events attempted=%d\n", sent.Load())
}

// This checks only the advertised expiry for test setup; the API validates signatures.
func accessTokenLasts(token string, deadline time.Time) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		ExpiresAt int64  `json:"exp"`
		Type      string `json:"type"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return false
	}
	return claims.Type == "access" && claims.ExpiresAt > deadline.Unix()
}

func queue(ctx context.Context, jobs chan<- task, job task) bool {
	select {
	case jobs <- job:
		return true
	case <-ctx.Done():
		return false
	}
}

func sendEvents(ctx context.Context, client *http.Client, base, token string, product uuid.UUID, query string, count int, sent *atomic.Int64) bool {
	events := make([]recommendation.InputEvent, 0, count)
	for i := 0; i < count; i++ {
		n := sent.Add(1)
		event := recommendation.InputEvent{EventID: uuid.New(), OccurredAt: time.Now().UTC(), Source: "load-test"}
		switch n % 10 {
		case 0:
			event.Type = recommendation.SearchSubmit
			event.Query = query
		case 1, 2, 3:
			event.Type = recommendation.ProductView
			event.ProductID = product
			event.DurationMS = 30000
		default:
			event.Type = recommendation.ProductClick
			event.ProductID = product
		}
		events = append(events, event)
	}
	body, _ := json.Marshal(map[string]any{"events": events})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/behavior/events", bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == 202
}

func readRecommendations(ctx context.Context, client *http.Client, base, token string, product uuid.UUID) bool {
	path := "/api/v1/recommendations/home?limit=12"
	if time.Now().UnixNano()%2 == 0 {
		path = "/api/v1/products/" + product.String() + "/recommendations?limit=12"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == 200
}
