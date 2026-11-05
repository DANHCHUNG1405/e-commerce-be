package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/productsearch"
	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Card struct {
	ID       uuid.UUID `json:"id"`
	SellerID uuid.UUID `json:"sellerId"`
	Name     string    `json:"name"`
	Slug     string    `json:"slug"`
	ImageURL string    `json:"imageUrl"`
	MinPrice int64     `json:"minPrice"`
}

type Result struct {
	RecommendationID uuid.UUID `json:"recommendationId"`
	Products         []Card    `json:"products"`
}

type Service struct {
	Repo   *Repository
	Redis  *redis.Client
	Search *productsearch.Client
}

type candidate struct {
	ID          uuid.UUID   `json:"id"`
	SellerID    uuid.UUID   `json:"sellerId"`
	Categories  []uuid.UUID `json:"categories"`
	Sales       int64       `json:"sales"`
	Rating      float64     `json:"rating"`
	ReviewCount int64       `json:"reviewCount"`
	CreatedAt   time.Time   `json:"createdAt"`
	Relevance   float64     `json:"relevance"`
}

func (s *Service) Home(ctx context.Context, userID uuid.UUID, limit int) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	result := Result{RecommendationID: uuid.New(), Products: []Card{}}
	profileCtx, profileCancel := context.WithTimeout(ctx, 60*time.Millisecond)
	interests, err := LoadInterests(profileCtx, s.Redis, userID, time.Now())
	profileCancel()
	if err != nil {
		interests = nil
	}
	if len(interests) > 3 {
		interests = interests[:3]
	}
	return s.recommend(ctx, userID, uuid.Nil, interests, limit, result)
}

func (s *Service) Similar(ctx context.Context, userID, productID uuid.UUID, limit int) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	result := Result{RecommendationID: uuid.New(), Products: []Card{}}
	product, err := s.Repo.Product(ctx, productID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, err
		}
		return result, nil
	}
	shop, err := shared.GetSellerInfo(ctx, product.SellerID)
	if err != nil {
		return result, nil
	}
	if shop.Status != "approved" {
		return result, gorm.ErrRecordNotFound
	}
	ids, err := s.Repo.Categories(ctx, productID)
	if err != nil {
		return result, nil
	}
	interests := make([]Interest, 0, len(ids))
	for _, id := range ids {
		interests = append(interests, Interest{CategoryID: id, Score: 2})
	}
	return s.recommend(ctx, userID, productID, interests, limit, result)
}

func (s *Service) recommend(ctx context.Context, userID, exclude uuid.UUID, interests []Interest, limit int, result Result) (Result, error) {
	cacheKey := rediskey.Key("recommendation", "result", userID.String(), exclude.String())
	var candidates []candidate
	if s.Redis != nil {
		cacheCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
		raw, err := s.Redis.Get(cacheCtx, cacheKey).Bytes()
		cancel()
		if err == nil {
			_ = json.Unmarshal(raw, &candidates)
		}
	}
	if len(candidates) == 0 {
		for _, interest := range interests {
			items, err := s.categoryCandidates(ctx, interest.CategoryID)
			if err != nil {
				break
			}
			candidates = append(candidates, items...)
		}
		if exclude != uuid.Nil {
			if source, err := s.Repo.ProductSource(ctx, exclude); err == nil {
				if items, err := s.similarCandidates(ctx, source); err == nil {
					candidates = append(candidates, items...)
				}
			}
		}
		if len(candidates) < limit*2 {
			popular, err := s.popularCandidates(ctx)
			if err == nil {
				candidates = append(candidates, popular...)
			}
		}
		if s.Redis != nil && len(candidates) > 0 {
			if raw, err := json.Marshal(candidates); err == nil {
				cacheCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
				_ = s.Redis.Set(cacheCtx, cacheKey, raw, 60*time.Second).Err()
				cancel()
			}
		}
	}
	unique := make(map[uuid.UUID]candidate, len(candidates))
	for _, item := range candidates {
		if item.ID != exclude && item.ID != uuid.Nil {
			if _, ok := unique[item.ID]; !ok {
				unique[item.ID] = item
			}
		}
	}
	ranked := make([]candidate, 0, len(unique))
	for _, item := range unique {
		ranked = append(ranked, item)
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := rank(ranked[i], interests, exclude != uuid.Nil), rank(ranked[j], interests, exclude != uuid.Nil)
		if a == b {
			return ranked[i].ID.String() < ranked[j].ID.String()
		}
		return a > b
	})
	ids := make([]uuid.UUID, 0, len(ranked))
	for _, item := range ranked {
		ids = append(ids, item.ID)
		if len(ids) >= 100 {
			break
		}
	}
	cards, err := s.hydrate(ctx, ids)
	if err != nil {
		return result, nil
	} // Shopping pages remain usable during a recommendation dependency outage.
	byID := make(map[uuid.UUID]Card, len(cards))
	for _, card := range cards {
		byID[card.ID] = card
	}
	sellerCount := map[uuid.UUID]int{}
	for _, id := range ids {
		card, ok := byID[id]
		if !ok || sellerCount[card.SellerID] >= 2 {
			continue
		}
		result.Products = append(result.Products, card)
		sellerCount[card.SellerID]++
		if len(result.Products) == limit {
			break
		}
	}
	return result, nil
}

func rank(item candidate, interests []Interest, similar bool) float64 {
	score := item.Relevance
	for _, interest := range interests {
		for _, id := range item.Categories {
			if id == interest.CategoryID {
				if similar {
					score += 5
				} else {
					score += 3 * math.Log1p(interest.Score)
				}
				break
			}
		}
	}
	bayesian := (item.Rating*float64(item.ReviewCount) + 3.5*10) / float64(item.ReviewCount+10)
	score += bayesian*0.5 + math.Log1p(float64(item.Sales))*0.25
	if !item.CreatedAt.IsZero() {
		score += math.Exp(-math.Max(0, time.Since(item.CreatedAt).Hours())/(24*30)) * 0.5
	}
	return score
}

func (s *Service) categoryCandidates(ctx context.Context, id uuid.UUID) ([]candidate, error) {
	key := rediskey.Key("recommendation", "candidates", "category", id.String())
	if items, ok := s.cachedCandidates(ctx, key); ok {
		return items, nil
	}
	if s.Search == nil {
		return nil, fmt.Errorf("Elasticsearch unavailable")
	}
	query := productsearch.BuildQuery("", nil, &id, nil, nil)
	searchCtx, cancel := context.WithTimeout(ctx, 180*time.Millisecond)
	defer cancel()
	docs, err := s.Search.SearchDocuments(searchCtx, query, 60)
	if err != nil {
		return nil, err
	}
	items := docsToCandidates(docs)
	s.cacheCandidates(ctx, key, items)
	return items, nil
}

func (s *Service) similarCandidates(ctx context.Context, product models.Product) ([]candidate, error) {
	if s.Search == nil {
		return nil, fmt.Errorf("Elasticsearch unavailable")
	}
	query := map[string]any{"bool": map[string]any{"filter": []any{map[string]any{"term": map[string]any{"status": "published"}}}, "should": []any{map[string]any{"more_like_this": map[string]any{"fields": []string{"name", "description"}, "like": []string{product.Name, product.Description}, "min_term_freq": 1, "min_doc_freq": 1}}}, "minimum_should_match": 1}}
	searchCtx, cancel := context.WithTimeout(ctx, 180*time.Millisecond)
	defer cancel()
	docs, err := s.Search.SearchDocuments(searchCtx, query, 50)
	if err != nil {
		return nil, err
	}
	items := docsToCandidates(docs)
	for i := range items {
		items[i].Relevance = 2.0 / (1 + float64(i)/10)
	}
	return items, nil
}

func (s *Service) popularCandidates(ctx context.Context) ([]candidate, error) {
	key := rediskey.Key("recommendation", "candidates", "popular")
	if items, ok := s.cachedCandidates(ctx, key); ok {
		return items, nil
	}
	if s.Search != nil {
		query := productsearch.BuildQuery("", nil, nil, nil, nil)
		searchCtx, cancel := context.WithTimeout(ctx, 180*time.Millisecond)
		docs, err := s.Search.SearchDocuments(searchCtx, query, 80, productsearch.BuildSort("best_selling")...)
		cancel()
		if err == nil {
			items := docsToCandidates(docs)
			s.cacheCandidates(ctx, key, items)
			return items, nil
		}
	}
	// A bounded PostgreSQL fallback is used only when the search index is unavailable.
	items, err := s.Repo.RecentFallback(ctx)
	if err != nil {
		return nil, err
	}
	s.cacheCandidates(ctx, key, items)
	return items, nil
}

func docsToCandidates(docs []productsearch.Document) []candidate {
	items := make([]candidate, 0, len(docs))
	for i, d := range docs {
		created, _ := time.Parse(time.RFC3339Nano, d.CreatedAt)
		items = append(items, candidate{ID: d.ID, SellerID: d.SellerID, Categories: d.CategoryIDs, Sales: d.Sales, Rating: d.Rating, ReviewCount: d.ReviewCount, CreatedAt: created, Relevance: 1 / (1 + float64(i)/30)})
	}
	return items
}

func (s *Service) cachedCandidates(ctx context.Context, key string) ([]candidate, bool) {
	if s.Redis == nil {
		return nil, false
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	raw, err := s.Redis.Get(cacheCtx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var items []candidate
	if json.Unmarshal(raw, &items) != nil {
		return nil, false
	}
	return items, true
}
func (s *Service) cacheCandidates(ctx context.Context, key string, items []candidate) {
	if s.Redis == nil {
		return
	}
	if raw, err := json.Marshal(items); err == nil {
		cacheCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		_ = s.Redis.Set(cacheCtx, key, raw, 5*time.Minute).Err()
	}
}

func (s *Service) hydrate(ctx context.Context, ids []uuid.UUID) ([]Card, error) {
	if len(ids) == 0 {
		return []Card{}, nil
	}
	cards, err := s.Repo.Cards(ctx, ids)
	if err != nil {
		return nil, err
	}
	sellers := make([]uuid.UUID, 0, len(cards))
	seenSellers := make(map[uuid.UUID]bool, len(cards))
	for _, card := range cards {
		if !seenSellers[card.SellerID] {
			sellers = append(sellers, card.SellerID)
			seenSellers[card.SellerID] = true
		}
	}
	if len(sellers) == 0 {
		return []Card{}, nil
	}
	shops, err := shared.BatchSellerInfo(ctx, sellers)
	if err != nil {
		return nil, err
	}
	approved := make([]Card, 0, len(cards))
	for _, card := range cards {
		if shops[card.SellerID].Status == "approved" {
			approved = append(approved, card)
		}
	}
	return approved, nil
}
