#!/bin/bash
# One MySQL login per service that can read and write its own database and nothing else:
# no schema changes, no other service's data. Schemas are applied by root beforehand.
# The mysql container runs this on first start (empty volume); CI runs it with MYSQL_HOST set.
set -eo pipefail
: "${MYSQL_ROOT_PASSWORD:?}" "${USER_DB_PASSWORD:?}" "${PRODUCT_DB_PASSWORD:?}" "${ORDER_DB_PASSWORD:?}"
# These go straight into the SQL below (run as root), so allow nothing that could break out of the quotes.
for pw in "$USER_DB_PASSWORD" "$PRODUCT_DB_PASSWORD" "$ORDER_DB_PASSWORD"; do
  [[ $pw =~ ^[A-Za-z0-9-]+$ ]] || { echo "DB passwords may only use letters, digits and dashes" >&2; exit 1; }
done

MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql ${MYSQL_HOST:+-h "$MYSQL_HOST"} -uroot <<SQL
CREATE USER IF NOT EXISTS 'user_svc'@'%' IDENTIFIED BY '${USER_DB_PASSWORD}';
GRANT SELECT, INSERT, UPDATE, DELETE ON users_db.* TO 'user_svc'@'%';

CREATE USER IF NOT EXISTS 'product_svc'@'%' IDENTIFIED BY '${PRODUCT_DB_PASSWORD}';
GRANT SELECT, INSERT, UPDATE, DELETE ON products_db.* TO 'product_svc'@'%';

CREATE USER IF NOT EXISTS 'order_svc'@'%' IDENTIFIED BY '${ORDER_DB_PASSWORD}';
GRANT SELECT, INSERT, UPDATE, DELETE ON orders_db.* TO 'order_svc'@'%';
SQL
