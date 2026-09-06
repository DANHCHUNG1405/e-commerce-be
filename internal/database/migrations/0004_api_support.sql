ALTER TABLE sellers ADD COLUMN status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','suspended'));
ALTER TABLE orders ADD COLUMN address_snapshot jsonb NOT NULL DEFAULT '{}';
ALTER TABLE orders ADD COLUMN idempotency_key text;
CREATE UNIQUE INDEX orders_user_idempotency ON orders(user_id,idempotency_key);
CREATE INDEX products_seller ON products(seller_id);
CREATE INDEX variants_product ON product_variants(product_id);
CREATE INDEX seller_orders_order ON seller_orders(order_id);
CREATE INDEX order_items_seller_order ON order_items(seller_order_id);
CREATE INDEX orders_user_created ON orders(user_id,created_at);
