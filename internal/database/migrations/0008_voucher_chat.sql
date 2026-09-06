ALTER TABLE coupons ADD COLUMN active boolean NOT NULL DEFAULT true;
ALTER TABLE coupons ADD COLUMN max_discount bigint NOT NULL DEFAULT 0 CHECK (max_discount >= 0);
ALTER TABLE orders ADD COLUMN coupon_code text NOT NULL DEFAULT '';
ALTER TABLE seller_orders ADD COLUMN discount bigint NOT NULL DEFAULT 0 CHECK (discount >= 0);
-- NOT VALID preserves legacy rows but enforces constraints on new/updated data.
ALTER TABLE coupons ADD CONSTRAINT coupon_valid_window CHECK (ends_at > starts_at) NOT VALID;
ALTER TABLE coupons ADD CONSTRAINT coupon_valid_quota CHECK (usage_limit >= 0 AND used_count >= 0 AND (usage_limit = 0 OR used_count <= usage_limit)) NOT VALID;
ALTER TABLE coupons ADD CONSTRAINT coupon_valid_value CHECK ((type='fixed' AND value>0) OR (type='percent' AND value BETWEEN 1 AND 100 AND max_discount>0)) NOT VALID;
ALTER TABLE coupon_rules ADD CONSTRAINT coupon_minimum_nonnegative CHECK (min_order >= 0) NOT VALID;
CREATE INDEX coupon_active_window_idx ON coupons (starts_at,ends_at) WHERE active AND deleted_at IS NULL;

CREATE TABLE chat_conversations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id uuid NOT NULL REFERENCES users(id),
    seller_id uuid NOT NULL REFERENCES sellers(id),
    last_sequence bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (buyer_id,seller_id)
);
CREATE INDEX chat_conversations_seller_idx ON chat_conversations (seller_id,updated_at DESC);
CREATE TABLE chat_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES chat_conversations(id),
    sender_id uuid NOT NULL REFERENCES users(id),
    client_message_id uuid NOT NULL,
    sequence bigint NOT NULL CHECK (sequence > 0),
    body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 5000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (conversation_id,sender_id,client_message_id),
    UNIQUE (conversation_id,sequence)
);
CREATE TABLE chat_reads (
    conversation_id uuid NOT NULL REFERENCES chat_conversations(id),
    user_id uuid NOT NULL REFERENCES users(id),
    last_sequence bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id,user_id)
);
