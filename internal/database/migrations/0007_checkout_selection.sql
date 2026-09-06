-- Empty string denotes legacy whole-cart checkout. Selected UUIDs are sorted.
ALTER TABLE orders ADD COLUMN checkout_selection text NOT NULL DEFAULT '';
