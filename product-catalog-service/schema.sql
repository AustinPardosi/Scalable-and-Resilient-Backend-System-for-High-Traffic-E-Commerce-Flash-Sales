CREATE DATABASE IF NOT EXISTS products_db;

USE products_db;

CREATE TABLE IF NOT EXISTS products (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    price_cents BIGINT NOT NULL CHECK (price_cents >= 0),
    -- Last line of defense: even buggy code cannot sell stock that does not exist.
    stock       INT NOT NULL CHECK (stock >= 0)
);

-- One row per order. RELEASED is final: a late or retried reserve for that order is refused.
CREATE TABLE IF NOT EXISTS reservations (
    order_id   BIGINT PRIMARY KEY,
    status     ENUM('RESERVED', 'RELEASED') NOT NULL,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE TABLE IF NOT EXISTS reservation_items (
    order_id   BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    quantity   INT NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (order_id, product_id)
);

INSERT IGNORE INTO products (id, name, description, price_cents, stock) VALUES
    (1, 'Limited Sneakers', 'Flash sale item: 100 pairs only', 9999, 100),
    (2, 'Everyday T-Shirt', 'Plenty in stock', 1999, 1000);
