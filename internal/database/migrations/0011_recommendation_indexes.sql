CREATE INDEX IF NOT EXISTS product_categories_category_product_idx ON product_categories (category_id, product_id);
CREATE INDEX IF NOT EXISTS product_variants_available_product_idx ON product_variants (product_id, price) WHERE deleted_at IS NULL AND stock > 0;
CREATE INDEX IF NOT EXISTS product_images_product_sort_idx ON product_images (product_id, sort_order) WHERE deleted_at IS NULL;
