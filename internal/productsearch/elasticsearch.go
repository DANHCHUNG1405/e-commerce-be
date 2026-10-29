package productsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/google/uuid"
)

const Alias = "products-current"

type Client struct{ es *database.ElasticsearchClient }

func New(es *database.ElasticsearchClient) *Client { return &Client{es: es} }

type Variant struct {
	SKU   string `json:"sku"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
}
type Document struct {
	ID          uuid.UUID   `json:"id"`
	SellerID    uuid.UUID   `json:"seller_id"`
	SellerName  string      `json:"seller_name"`
	Status      string      `json:"status"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	SKU         []string    `json:"sku"`
	Variants    []Variant   `json:"variants"`
	CategoryIDs []uuid.UUID `json:"category_ids"`
	Categories  []string    `json:"categories"`
	MinPrice    *int64      `json:"min_price,omitempty"`
	Sales       int64       `json:"sales"`
	Rating      float64     `json:"rating"`
	CreatedAt   string      `json:"created_at"`
	Suggest     []string    `json:"suggest"`
}

func BuildQuery(q string, seller, category *uuid.UUID, minPrice, maxPrice *int64) map[string]any {
	filters := []any{map[string]any{"term": map[string]any{"status": "published"}}}
	if seller != nil {
		filters = append(filters, map[string]any{"term": map[string]any{"seller_id": seller.String()}})
	}
	if category != nil {
		filters = append(filters, map[string]any{"term": map[string]any{"category_ids": category.String()}})
	}
	if minPrice != nil || maxPrice != nil {
		bounds := map[string]any{}
		if minPrice != nil {
			bounds["gte"] = *minPrice
		}
		if maxPrice != nil {
			bounds["lte"] = *maxPrice
		}
		filters = append(filters, map[string]any{"nested": map[string]any{"path": "variants", "query": map[string]any{"range": map[string]any{"variants.price": bounds}}}})
	}
	must := []any{}
	if strings.TrimSpace(q) != "" {
		must = append(must, map[string]any{"bool": map[string]any{"should": []any{
			map[string]any{"match": map[string]any{"name": map[string]any{"query": q, "boost": 4}}},
			map[string]any{"fuzzy": map[string]any{"name": map[string]any{"value": q, "fuzziness": "AUTO", "prefix_length": 1, "max_expansions": 25, "boost": 2}}},
			map[string]any{"nested": map[string]any{"path": "variants", "query": map[string]any{"match": map[string]any{"variants.name": q}}}},
			map[string]any{"match": map[string]any{"description": q}},
			map[string]any{"match": map[string]any{"seller_name": q}},
			map[string]any{"match": map[string]any{"categories": q}},
			map[string]any{"term": map[string]any{"sku": map[string]any{"value": q, "boost": 5}}},
		}, "minimum_should_match": 1}})
	}
	return map[string]any{"bool": map[string]any{"filter": filters, "must": must}}
}

func BuildSort(kind string) []any {
	switch kind {
	case "price_asc":
		return []any{map[string]any{"min_price": map[string]any{"order": "asc", "missing": "_last"}}, map[string]any{"created_at": "desc"}, map[string]any{"id": "asc"}}
	case "price_desc":
		return []any{map[string]any{"min_price": map[string]any{"order": "desc", "missing": "_last"}}, map[string]any{"created_at": "desc"}, map[string]any{"id": "asc"}}
	case "best_selling":
		return []any{map[string]any{"sales": "desc"}, map[string]any{"created_at": "desc"}, map[string]any{"id": "asc"}}
	case "rating":
		return []any{map[string]any{"rating": "desc"}, map[string]any{"created_at": "desc"}, map[string]any{"id": "asc"}}
	case "relevance":
		return []any{"_score", map[string]any{"created_at": "desc"}, map[string]any{"id": "asc"}}
	default:
		return []any{map[string]any{"created_at": "desc"}, map[string]any{"id": "asc"}}
	}
}

func Mapping() map[string]any {
	return map[string]any{
		"settings": map[string]any{"analysis": map[string]any{"analyzer": map[string]any{
			"vi_folded": map[string]any{"tokenizer": "standard", "filter": []string{"lowercase", "asciifolding"}},
			"vi_prefix": map[string]any{"tokenizer": "standard", "filter": []string{"lowercase", "asciifolding"}},
		}}},
		"mappings": map[string]any{"properties": map[string]any{
			"id": map[string]any{"type": "keyword"}, "seller_id": map[string]any{"type": "keyword"}, "status": map[string]any{"type": "keyword"},
			"name":        map[string]any{"type": "text", "analyzer": "vi_folded", "fields": map[string]any{"keyword": map[string]any{"type": "keyword"}}},
			"seller_name": map[string]any{"type": "text", "analyzer": "vi_folded"},
			"categories":  map[string]any{"type": "text", "analyzer": "vi_folded"},
			"description": map[string]any{"type": "text", "analyzer": "vi_folded"}, "sku": map[string]any{"type": "keyword"},
			"variants":     map[string]any{"type": "nested", "properties": map[string]any{"sku": map[string]any{"type": "keyword"}, "name": map[string]any{"type": "text", "analyzer": "vi_folded"}, "price": map[string]any{"type": "long"}}},
			"category_ids": map[string]any{"type": "keyword"}, "min_price": map[string]any{"type": "long"}, "sales": map[string]any{"type": "long"},
			"rating": map[string]any{"type": "double"}, "created_at": map[string]any{"type": "date"},
			"suggest": map[string]any{"type": "completion", "analyzer": "vi_prefix", "preserve_separators": true, "preserve_position_increments": false},
		}},
	}
}

func (c *Client) Search(ctx context.Context, query map[string]any, sort []any, from, size int) ([]uuid.UUID, error) {
	body, _ := json.Marshal(map[string]any{"from": from, "size": size, "query": query, "sort": sort, "_source": []string{"id"}})
	resp, err := c.es.Do(ctx, http.MethodPost, "/"+Alias+"/_search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Hits struct {
			Hits []struct {
				Source struct {
					ID uuid.UUID `json:"id"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Elasticsearch search: %w", err)
	}
	out := make([]uuid.UUID, 0, len(result.Hits.Hits))
	for _, h := range result.Hits.Hits {
		if h.Source.ID != uuid.Nil {
			out = append(out, h.Source.ID)
		}
	}
	return out, nil
}

func (c *Client) Suggest(ctx context.Context, q string) ([]string, error) {
	body, _ := json.Marshal(map[string]any{"suggest": map[string]any{"products": map[string]any{"prefix": q, "completion": map[string]any{"field": "suggest", "size": 5, "skip_duplicates": true}}}})
	resp, err := c.es.Do(ctx, http.MethodPost, "/"+Alias+"/_search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Suggest map[string][]struct {
			Options []struct {
				Text string `json:"text"`
			} `json:"options"`
		} `json:"suggest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Elasticsearch suggestions: %w", err)
	}
	out := []string{}
	for _, entries := range result.Suggest {
		for _, entry := range entries {
			for _, option := range entry.Options {
				if len(out) < 5 {
					out = append(out, option.Text)
				}
			}
		}
	}
	return out, nil
}
func (c *Client) Index(ctx context.Context, doc Document) error { return c.IndexTo(ctx, Alias, doc) }
func (c *Client) IndexTo(ctx context.Context, index string, doc Document) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	resp, err := c.es.Do(ctx, http.MethodPut, "/"+url.PathEscape(index)+"/_doc/"+url.PathEscape(doc.ID.String()), bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
func (c *Client) Delete(ctx context.Context, id uuid.UUID) error {
	resp, err := c.es.Do(ctx, http.MethodDelete, "/"+Alias+"/_doc/"+url.PathEscape(id.String()), nil)
	if err != nil {
		if strings.Contains(err.Error(), "status 404") {
			return nil
		}
		return err
	}
	defer resp.Body.Close()
	return nil
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
func (c *Client) SetAlias(ctx context.Context, name string) error {
	remove := []any{}
	resp, err := c.es.Do(ctx, http.MethodGet, "/_alias/"+url.PathEscape(Alias), nil)
	if err == nil {
		var current map[string]json.RawMessage
		decodeErr := json.NewDecoder(resp.Body).Decode(&current)
		resp.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode Elasticsearch alias: %w", decodeErr)
		}
		for index := range current {
			remove = append(remove, map[string]any{"remove": map[string]any{"index": index, "alias": Alias}})
		}
	} else if !strings.Contains(err.Error(), "status 404") {
		return err
	}
	actions := append(remove, map[string]any{"add": map[string]any{"index": name, "alias": Alias}})
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
		return fmt.Errorf("decode Elasticsearch alias update: %w", err)
	}
	if result.Errors {
		return fmt.Errorf("Elasticsearch alias update reported failed actions")
	}
	return nil
}
