CREATE UNIQUE INDEX review_order_item_once ON reviews(order_item_id) WHERE order_item_id IS NOT NULL;
