import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import exec from 'k6/execution';

const baseURL = (__ENV.BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const s3URL = (__ENV.S3_URL || 'http://127.0.0.1:8081').replace(/\/$/, '');
const adminToken = __ENV.ADMIN_TOKEN || 'dev-admin-token';
const buckets = (__ENV.BUCKETS || 'maxio-a,maxio-b,maxio-c')
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean);
const objectBytes = Number.parseInt(__ENV.S3_OBJECT_BYTES || '1024', 10);

export const options = {
  vus: Number.parseInt(__ENV.VUS || '4', 10),
  duration: __ENV.DURATION || '30s',
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<750'],
    maxio_control_ok: ['rate>0.95'],
    maxio_s3_ok: ['rate>0.90'],
  },
};

const controlOK = new Rate('maxio_control_ok');
const s3OK = new Rate('maxio_s3_ok');
const s3Objects = new Counter('maxio_s3_objects');

function adminHeaders(extra = {}) {
  return {
    headers: {
      Authorization: `Bearer ${adminToken}`,
      'X-Maxio-Control': adminToken,
      ...extra,
    },
  };
}

function objectBody() {
  const seed = `${exec.vu.idInTest}-${exec.scenario.iterationInTest}`;
  const repeated = `${seed}:maxio-seaweed-k6:`;
  return repeated.repeat(Math.ceil(objectBytes / repeated.length)).slice(0, objectBytes);
}

function selectBucket() {
  if (buckets.length === 0) {
    return 'maxio-a';
  }
  return buckets[exec.scenario.iterationInTest % buckets.length];
}

function recordControl(response) {
  const ok = response.status >= 200 && response.status < 300;
  controlOK.add(ok);
  return ok;
}

function recordS3(response) {
  const ok = response.status >= 200 && response.status < 300;
  s3OK.add(ok);
  return ok;
}

export default function () {
  group('control-plane', () => {
    check(http.get(`${baseURL}/healthz`), { 'healthz is 200': (r) => recordControl(r) && r.status === 200 });
    check(http.get(`${baseURL}/readyz`), { 'readyz is 200': (r) => recordControl(r) && r.status === 200 });
    check(http.get(`${baseURL}/metrics`, adminHeaders()), { 'metrics is 200': (r) => recordControl(r) && r.status === 200 });
    check(http.get(`${baseURL}/_s3/upstreams`, adminHeaders()), { 'upstreams is 200': (r) => recordControl(r) && r.status === 200 });
    check(http.get(`${baseURL}/_index/status`, adminHeaders()), { 'index status is 200': (r) => recordControl(r) && r.status === 200 });
  });

  group('s3-proxy', () => {
    const bucket = selectBucket();
    const key = `k6/${exec.vu.idInTest}/${exec.scenario.iterationInTest}.txt`;
    const url = `${s3URL}/${bucket}/${key}`;
    const body = objectBody();
    const put = http.put(url, body, { headers: { 'Content-Type': 'text/plain' } });
    check(put, { 'put object accepted': (r) => recordS3(r) && [200, 201, 204].includes(r.status) });

    const get = http.get(url);
    check(get, { 'get object accepted': (r) => recordS3(r) && r.status === 200 });

    const del = http.del(url);
    check(del, { 'delete object accepted': (r) => recordS3(r) && [200, 202, 204, 404].includes(r.status) });
    s3Objects.add(1);
  });

  sleep(1);
}
