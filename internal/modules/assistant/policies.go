package assistant

import (
	"context"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

type PolicyChunk struct {
	Title   string `gorm:"column:title"`
	Content string `gorm:"column:content"`
	Status  string `gorm:"column:status"`
}

type PolicyRepository struct{ db *gorm.DB }

func NewPolicyRepository(db *gorm.DB) *PolicyRepository {
	return &PolicyRepository{db: db}
}

func (r *PolicyRepository) Search(ctx context.Context, query string) ([]PolicyChunk, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 1000 {
		return nil, nil
	}
	terms := policyTerms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	tsQuery := strings.Join(terms, ":* | ") + ":*"
	var passages []PolicyChunk
	err := r.db.WithContext(ctx).Raw(`
		SELECT title, content, status
		FROM assistant.policy_chunks
		WHERE status IN ('draft', 'approved')
		  AND effective_from <= now()
		  AND to_tsvector('simple', title || ' ' || content || ' ' || keywords) @@ to_tsquery('simple', ?)
		ORDER BY ts_rank_cd(to_tsvector('simple', title || ' ' || content || ' ' || keywords), to_tsquery('simple', ?)) DESC,
		         title ASC
		LIMIT 4`, tsQuery, tsQuery).Scan(&passages).Error
	return passages, err
}

func policyTerms(query string) []string {
	stopWords := map[string]struct{}{
		"tôi": {}, "mình": {}, "cho": {}, "hỏi": {}, "về": {}, "thế": {}, "nào": {}, "có": {},
		"là": {}, "gì": {}, "của": {}, "và": {}, "được": {}, "không": {}, "bao": {}, "nhiêu": {},
		"muốn": {}, "chính": {}, "sách": {}, "sản": {}, "phẩm": {}, "đơn": {}, "hàng": {},
		"thời": {}, "gian": {}, "cửa": {}, "hỗ": {}, "trợ": {},
		"how": {}, "what": {}, "the": {}, "is": {}, "do": {}, "i": {}, "my": {}, "can": {},
	}
	seen := make(map[string]struct{})
	terms := make([]string, 0, 12)
	for _, term := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(term)) < 2 {
			continue
		}
		if _, skip := stopWords[term]; skip {
			continue
		}
		if _, exists := seen[term]; exists {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
		if len(terms) == 12 {
			break
		}
	}
	return terms
}
