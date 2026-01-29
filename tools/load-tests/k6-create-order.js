import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 10,
  duration: '30s',
};

export default function () {
  const url = 'http://localhost:8081/v1/orders';
  const payload = JSON.stringify({
    user_id: 'u_' + __VU,
    items: [{ sku: 'SKU-RED-1', qty: 1, unit_price: 1999 }],
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': 'k6-' + __VU + '-' + __ITER,
    },
  };

  const res = http.post(url, payload, params);
  check(res, { 'status is 200/201': (r) => r.status === 200 || r.status === 201 });
  sleep(0.2);
}
