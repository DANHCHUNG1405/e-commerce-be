CREATE TABLE IF NOT EXISTS behavior_events (
    event_id UUID,
    user_id UUID,
    type LowCardinality(String),
    product_id UUID,
    product_ids Array(UUID),
    category_ids Array(UUID),
    duration_ms UInt32,
    query_hash String,
    recommendation_id UUID,
    source LowCardinality(String),
    occurred_at DateTime64(3, 'UTC'),
    received_at DateTime64(3, 'UTC')
) ENGINE = ReplacingMergeTree(received_at)
PARTITION BY toYYYYMM(received_at)
ORDER BY event_id
TTL received_at + INTERVAL 30 DAY DELETE;
