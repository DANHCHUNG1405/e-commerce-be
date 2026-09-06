package catalog

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"gorm.io/gorm"
)

type MetadataRepository struct{ collection *mongo.Collection }

func NewMetadataRepository(db *mongo.Database) *MetadataRepository {
	return &MetadataRepository{collection: db.Collection("product_metadata")}
}

type Metadata struct {
	ProductID      string         `bson:"_id" json:"productId"`
	Specifications map[string]any `bson:"specifications" json:"specifications"`
	SEO            map[string]any `bson:"seo" json:"seo"`
}

func (r *MetadataRepository) Get(ctx context.Context, id uuid.UUID) (Metadata, error) {
	var v Metadata
	err := r.collection.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&v)
	if err == mongo.ErrNoDocuments {
		err = gorm.ErrRecordNotFound
	}
	return v, err
}
func (r *MetadataRepository) Put(ctx context.Context, id uuid.UUID, v Metadata) error {
	v.ProductID = id.String()
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": id.String()}, v, options.Replace().SetUpsert(true))
	return err
}
func (s *Service) GetMetadata(ctx context.Context, r *MetadataRepository, id uuid.UUID) (Metadata, error) {
	if _, err := s.Detail(ctx, id); err != nil {
		return Metadata{}, err
	}
	return r.Get(ctx, id)
}
func (s *Service) PutMetadata(ctx context.Context, r *MetadataRepository, user, seller, id uuid.UUID, v Metadata) error {
	if err := s.repo.Seller(ctx, user, seller); err != nil {
		return err
	}
	var p models.Product
	if err := s.repo.One(ctx, &p, "id=? AND seller_id=? AND deleted_at IS NULL", id, seller); err != nil {
		return err
	}
	return r.Put(ctx, id, v)
}
