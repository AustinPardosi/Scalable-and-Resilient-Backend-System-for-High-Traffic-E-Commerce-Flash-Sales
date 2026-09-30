CREATE DATABASE IF NOT EXISTS orders_db;

USE orders_db;

CREATE TABLE IF NOT EXISTS orders (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    idempotency_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL, -- case-sensitive: "abC" != "ABc"
    status          ENUM('PENDING', 'CONFIRMED', 'PAID', 'CANCELED') NOT NULL,
    cancel_reason   VARCHAR(32) NOT NULL DEFAULT '',
    total_cents     BIGINT NOT NULL,
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    -- Same key from the same user is the same order: double clicks and client retries can't double-buy.
    UNIQUE KEY uq_user_idempotency (user_id, idempotency_key),
    KEY idx_status_updated (status, updated_at) -- sweeper: stale PENDING and unpaid CONFIRMED orders
);

CREATE TABLE IF NOT EXISTS order_items (
    order_id         BIGINT NOT NULL,
    product_id       BIGINT NOT NULL,
    quantity         INT NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT NOT NULL,
    PRIMARY KEY (order_id, product_id)
);

-- Transactional outbox: an event is written in the same transaction as the state change it
-- describes, then relayed to Kafka. No lost events, and no events for rolled-back changes.
CREATE TABLE IF NOT EXISTS outbox (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    topic        VARCHAR(64) NOT NULL,
    msg_key      VARCHAR(64) NOT NULL,
    payload      JSON NOT NULL,
    created_at   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    published_at TIMESTAMP(3) NULL,
    KEY idx_unpublished (published_at, id)
);
