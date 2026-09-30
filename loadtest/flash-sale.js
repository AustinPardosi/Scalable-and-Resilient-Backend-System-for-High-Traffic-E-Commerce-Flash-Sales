// Flash sale: BUYERS purchase attempts race for STOCK units of one product, and every
// buyer double-clicks (two concurrent submits sharing one Idempotency-Key).
// Pass = exactly STOCK orders confirmed, both clicks always get the same order,
// and every response is "confirmed" or "sold out", never a server error.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const USERS = 'http://user-service:8080';
const ORDERS = 'http://order-service:8082';
const PRODUCT_ID = Number(__ENV.PRODUCT_ID || 1);
const STOCK = Number(__ENV.STOCK || 100);

const confirmed = new Counter('orders_confirmed');
const soldOut = new Counter('orders_sold_out');

// "Sold out" (409) is a correct answer here, not a failed request.
http.setResponseCallback(http.expectedStatuses(200, 201, 409));

export const options = {
  scenarios: {
    flash_sale: {
      executor: 'shared-iterations',
      vus: Number(__ENV.VUS || 300),
      iterations: Number(__ENV.BUYERS || 2000),
      maxDuration: '3m',
    },
  },
  thresholds: {
    checks: ['rate==1'],
    orders_confirmed: [`count==${STOCK}`],
  },
  setupTimeout: '120s',
};

export function setup() {
  for (const url of [USERS, ORDERS, 'http://product-service:8081', 'http://pricing-service:8083']) {
    for (let tries = 0; http.get(`${url}/healthz`).status !== 200; tries++) {
      if (tries > 60) throw new Error(`${url} never became healthy`);
      sleep(1);
    }
  }

  const params = { headers: { 'Content-Type': 'application/json' } };
  const creds = JSON.stringify({ username: 'buyer', email: `buyer-${Date.now()}@example.com`, password: 'flash-sale-pw' });
  const signup = http.post(`${USERS}/users`, creds, params);
  if (signup.status !== 201) throw new Error(`signup: ${signup.status} ${signup.body}`);
  const login = http.post(`${USERS}/login`, creds, params);
  if (login.status !== 200) throw new Error(`login: ${login.status} ${login.body}`);
  return { token: login.json('token') };
}

const orderID = (r) => {
  try {
    return r.json('id');
  } catch (e) {
    return undefined;
  }
};

export default function ({ token }) {
  const params = {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
      'Idempotency-Key': `${__VU}-${__ITER}`,
    },
  };
  const body = JSON.stringify({ items: [{ product_id: PRODUCT_ID, quantity: 1 }] });
  const [a, b] = http.batch([
    ['POST', `${ORDERS}/orders`, body, params],
    ['POST', `${ORDERS}/orders`, body, params],
  ]);

  check(a, {
    'confirmed (201) or sold out (409)': (r) => r.status === 201 || r.status === 409,
    'double click gets the same order': (r) =>
      r.status === b.status && orderID(r) !== undefined && orderID(r) === orderID(b),
  });
  if (a.status === 409) soldOut.add(1);
  if (a.status === 201) {
    confirmed.add(1);
    // Pay, as a real buyer would; unpaid orders get their stock released after 15 minutes.
    const pay = http.post(`${ORDERS}/orders/${orderID(a)}/pay`, null, { headers: { Authorization: `Bearer ${token}` } });
    check(pay, { 'confirmed order can be paid': (r) => r.status === 200 });
  }
}
