package database

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ElasticsearchClient is the shared HTTP client for the local Elasticsearch cluster.
type ElasticsearchClient struct {
	endpoint   string
	searchMode string
	http       *http.Client
}

// ConnectElasticsearch creates an Elasticsearch client. Availability is checked
// by requests that use the client so PostgreSQL mode can start without ES.
func ConnectElasticsearch(ctx context.Context, endpoint string, modes ...string) (*ElasticsearchClient, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, fmt.Errorf("invalid Elasticsearch endpoint")
	}

	client := &ElasticsearchClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		http:     &http.Client{Timeout: 5 * time.Second},
	}
	if len(modes) > 0 {
		client.searchMode = modes[0]
	}
	_ = ctx
	return client, nil
}

// Endpoint returns the validated cluster URL.
func (c *ElasticsearchClient) Endpoint() string   { return c.endpoint }
func (c *ElasticsearchClient) SearchMode() string { return c.searchMode }

// Do sends a JSON request to Elasticsearch and rejects non-2xx responses.
func (c *ElasticsearchClient) Do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return nil, fmt.Errorf("create Elasticsearch request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Elasticsearch: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("Elasticsearch returned status %d", resp.StatusCode)
	}
	return resp, nil
}

// Ping checks whether Elasticsearch is responding.
func (c *ElasticsearchClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return fmt.Errorf("create Elasticsearch health request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request Elasticsearch health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Elasticsearch health request returned status %d", resp.StatusCode)
	}
	return nil
}

// Close releases idle HTTP connections.
func (c *ElasticsearchClient) Close() {
	c.http.CloseIdleConnections()
}
