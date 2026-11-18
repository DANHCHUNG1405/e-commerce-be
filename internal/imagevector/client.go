package imagevector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/google/uuid"
)

const (
	Alias        = "product-images-current"
	ModelVersion = "openai-clip-vit-b-32"
)

type Client struct {
	es      *database.ElasticsearchClient
	clipURL string
	http    *http.Client
}

func New(es *database.ElasticsearchClient, clipURL string) *Client {
	return &Client{es: es, clipURL: strings.TrimRight(clipURL, "/"), http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) Embed(ctx context.Context, data []byte, contentType string) ([]float32, string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="image"; filename="query"`)
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err = part.Write(data); err != nil {
		return nil, "", err
	}
	if err = w.Close(); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.clipURL+"/v1/embeddings/images", &body)
	if err != nil {
		return nil, "", fmt.Errorf("create CLIP request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("call CLIP service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("CLIP service returned status %d", resp.StatusCode)
	}
	var result struct {
		Vector []float32 `json:"vector"`
		Model  string    `json:"model"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("decode CLIP response: %w", err)
	}
	if len(result.Vector) != 512 || result.Model == "" {
		return nil, "", fmt.Errorf("CLIP returned invalid embedding")
	}
	return result.Vector, result.Model, nil
}

func (c *Client) EnsureIndex(ctx context.Context, name string) error {
	body, _ := json.Marshal(Mapping())
	resp, err := c.es.Do(ctx, http.MethodPut, "/"+url.PathEscape(name), bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) AliasExists(ctx context.Context) (bool, error) {
	resp, err := c.es.Do(ctx, http.MethodGet, "/_alias/"+Alias, nil)
	if err == nil {
		resp.Body.Close()
		return true, nil
	}
	if !strings.Contains(err.Error(), "status 404") {
		return false, err
	}
	return false, nil
}

func Mapping() map[string]any {
	return map[string]any{"mappings": map[string]any{"properties": map[string]any{
		"product_id": map[string]any{"type": "keyword"}, "image_id": map[string]any{"type": "keyword"}, "model": map[string]any{"type": "keyword"},
		"vector": map[string]any{"type": "dense_vector", "dims": 512, "index": true, "similarity": "cosine"},
	}}}
}

func (c *Client) Index(ctx context.Context, index string, product, image uuid.UUID, model string, vector []float32) error {
	body, err := json.Marshal(map[string]any{"product_id": product, "image_id": image, "model": model, "vector": vector})
	if err != nil {
		return err
	}
	resp, err := c.es.Do(ctx, http.MethodPut, "/"+url.PathEscape(index)+"/_doc/"+image.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) DeleteProduct(ctx context.Context, product uuid.UUID) error {
	return c.DeleteProductFrom(ctx, Alias, product)
}

func (c *Client) DeleteProductFrom(ctx context.Context, index string, product uuid.UUID) error {
	body, _ := json.Marshal(map[string]any{"query": map[string]any{"term": map[string]any{"product_id": product.String()}}})
	resp, err := c.es.Do(ctx, http.MethodPost, "/"+url.PathEscape(index)+"/_delete_by_query?conflicts=proceed", bytes.NewReader(body))
	if err != nil {
		if strings.Contains(err.Error(), "status 404") {
			return nil
		}
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) Search(ctx context.Context, vector []float32, model string, k int) ([]uuid.UUID, error) {
	body, _ := json.Marshal(map[string]any{"size": k * 5, "knn": map[string]any{"field": "vector", "query_vector": vector, "k": k * 5, "num_candidates": max(k*20, 100), "filter": map[string]any{"term": map[string]any{"model": model}}}, "_source": []string{"product_id"}})
	resp, err := c.es.Do(ctx, http.MethodPost, "/"+Alias+"/_search", bytes.NewReader(body))
	if err != nil {
		if strings.Contains(err.Error(), "status 404") {
			return []uuid.UUID{}, nil
		}
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Hits struct {
			Hits []struct {
				Source struct {
					ProductID uuid.UUID `json:"product_id"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode image search: %w", err)
	}
	hitIDs := make([]uuid.UUID, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		hitIDs = append(hitIDs, hit.Source.ProductID)
	}
	return UniqueProductIDs(hitIDs, k), nil
}

func UniqueProductIDs(hitIDs []uuid.UUID, limit int) []uuid.UUID {
	ids := make([]uuid.UUID, 0, min(limit, len(hitIDs)))
	seen := make(map[uuid.UUID]bool, len(hitIDs))
	for _, id := range hitIDs {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if len(ids) == limit {
			break
		}
	}
	return ids
}

func (c *Client) SetAlias(ctx context.Context, index string) error {
	remove := []any{}
	resp, err := c.es.Do(ctx, http.MethodGet, "/_alias/"+Alias, nil)
	if err == nil {
		var current map[string]json.RawMessage
		decodeErr := json.NewDecoder(resp.Body).Decode(&current)
		resp.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		for name := range current {
			remove = append(remove, map[string]any{"remove": map[string]any{"index": name, "alias": Alias}})
		}
	} else if !strings.Contains(err.Error(), "status 404") {
		return err
	}
	actions := append(remove, map[string]any{"add": map[string]any{"index": index, "alias": Alias}})
	body, _ := json.Marshal(map[string]any{"actions": actions})
	resp, err = c.es.Do(ctx, http.MethodPost, "/_aliases", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result struct {
		Errors bool `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode image alias update: %w", err)
	}
	if result.Errors {
		return fmt.Errorf("Elasticsearch image alias update reported failed actions")
	}
	return nil
}
