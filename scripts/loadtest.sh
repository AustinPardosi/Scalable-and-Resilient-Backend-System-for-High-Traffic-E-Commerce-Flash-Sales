#!/usr/bin/env bash
# Flash-sale proof: resets product 1 to STOCK units, lets BUYERS (default 2000) race for
# them, then checks the database: exactly STOCK units sold, none left, nothing lost.
set -euo pipefail
cd "$(dirname "$0")/.."
export STOCK=${STOCK:-100}
PRODUCT_ID=1

[ -f .env ] || { echo "No .env: cp .env.example .env" >&2; exit 1; }
for n in "$STOCK" "${BUYERS:-0}" "${VUS:-0}"; do
  [[ $n =~ ^[0-9]+$ ]] || { echo "STOCK, BUYERS and VUS must be whole numbers" >&2; exit 1; }
done

# Credentials are read inside the containers, so they never touch this shell or ps.
sql() { docker compose exec -T mysql sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -N -B -e "$1"' sh "$1"; }

docker compose up -d --build
tries=0
until sql "SELECT 1" >/dev/null 2>&1; do
  (( ++tries < 60 )) || { echo "MySQL did not come up in 2 minutes" >&2; exit 1; }
  sleep 2
done

sql "UPDATE products_db.products SET stock = $STOCK WHERE id = $PRODUCT_ID"
docker compose exec -T redis redis-cli DEL "price:$PRODUCT_ID" >/dev/null # drop the price cached at the old stock level
start=$(sql "SELECT NOW(3)")

docker compose --profile loadtest run --rm -e STOCK -e BUYERS -e VUS k6

sold=$(sql "SELECT COALESCE(SUM(i.quantity), 0) FROM orders_db.orders o
            JOIN orders_db.order_items i ON i.order_id = o.id
            WHERE i.product_id = $PRODUCT_ID AND o.status IN ('CONFIRMED', 'PAID') AND o.created_at >= '$start'")
left=$(sql "SELECT stock FROM products_db.products WHERE id = $PRODUCT_ID")

echo "sold $sold of $STOCK, stock left: $left"
if [ "$sold" -eq "$STOCK" ] && [ "$left" -eq 0 ]; then
  echo "PASS: no overselling, no lost stock"
else
  echo "FAIL"
  exit 1
fi
