import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import exec from 'k6/execution';
import crypto from 'k6/crypto';

const baseURL = (__ENV.BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const s3URL = (__ENV.S3_URL || 'http://127.0.0.1:8081').replace(/\/$/, '');
const adminToken = __ENV.ADMIN_TOKEN || 'dev-admin-token';
const buckets = (__ENV.BUCKETS || 'maxio-a,maxio-b,maxio-c')
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean);
const objectBytes = Number.parseInt(__ENV.S3_OBJECT_BYTES || '1024', 10);
const processingRecordCheck = boolEnv(__ENV.PROCESSING_RECORD_CHECK);
const processingRecordRetries = Math.max(0, Number.parseInt(__ENV.PROCESSING_RECORD_RETRIES || '5', 10) || 0);
const processingRecordRetrySleep = Math.max(0, Number.parseFloat(__ENV.PROCESSING_RECORD_RETRY_SLEEP || '0.2') || 0);
const processingRecordListStatus = (__ENV.PROCESSING_RECORD_LIST_STATUS || '').trim();
const processingRecordListLimit = Math.max(1, Number.parseInt(__ENV.PROCESSING_RECORD_LIST_LIMIT || '10', 10) || 10);
const expectedProcessingCapabilities = (__ENV.PROCESSING_EXPECT_CAPABILITIES || '')
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean);
const expectedProcessingProcessors = (__ENV.PROCESSING_EXPECT_PROCESSORS || '')
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean);
const expectedProcessingProcessorModes = (__ENV.PROCESSING_EXPECT_PROCESSOR_MODES || '')
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean)
  .map((value) => {
    const separator = value.indexOf(':');
    if (separator < 0) {
      return { processor: value, mode: '', valid: false };
    }
    const processor = value.slice(0, separator).trim();
    const mode = value.slice(separator + 1).trim();
    return { processor, mode, valid: processor !== '' && mode !== '' };
  });
const expectedProcessingProcessorFailOpen = (__ENV.PROCESSING_EXPECT_PROCESSOR_FAIL_OPEN || '')
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean)
  .map((value) => {
    const separator = value.indexOf(':');
    if (separator < 0) {
      return { processor: value, failOpen: null, valid: false };
    }
    const processor = value.slice(0, separator).trim();
    const failOpen = parseBoolExpectation(value.slice(separator + 1));
    return { processor, failOpen, valid: processor !== '' && failOpen !== null };
  })
  .filter(Boolean);
const expectedProcessingResultMetadata = (__ENV.PROCESSING_EXPECT_RESULT_METADATA || '')
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean)
  .map((value) => {
    const separator = value.indexOf(':');
    if (separator < 0) {
      return { processor: value, key: '', expected: null, valid: false };
    }
    const processor = value.slice(0, separator).trim();
    const expectation = value.slice(separator + 1).trim();
    const equals = expectation.indexOf('=');
    const key = equals >= 0 ? expectation.slice(0, equals).trim() : expectation;
    const expected = equals >= 0 ? expectation.slice(equals + 1).trim() : null;
    return { processor, key, expected, valid: processor !== '' && key !== '' };
  });
const invalidProcessingExpectationCount =
  expectedProcessingProcessorModes.filter(({ valid }) => !valid).length +
  expectedProcessingProcessorFailOpen.filter(({ valid }) => !valid).length +
  expectedProcessingResultMetadata.filter(({ valid }) => !valid).length;
const clamavBlockCheck = boolEnv(__ENV.CLAMAV_BLOCK_CHECK);
const eicarBody = 'X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*';
const eicarDigest = `sha256:${crypto.sha256(eicarBody, 'hex')}`;

const expectedHTTPStatuses = [{ min: 200, max: 399 }];
if (clamavBlockCheck) {
  expectedHTTPStatuses.push(403, 423);
}
if (processingRecordCheck || clamavBlockCheck) {
  expectedHTTPStatuses.push(404);
}
http.setResponseCallback(http.expectedStatuses(...expectedHTTPStatuses));

export const options = {
  vus: Number.parseInt(__ENV.VUS || '4', 10),
  duration: __ENV.DURATION || '30s',
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<750'],
    maxio_control_ok: ['rate>0.95'],
    maxio_s3_ok: ['rate>0.90'],
    maxio_processing_expectation_config_ok: ['rate>0.99'],
  },
};

const controlOK = new Rate('maxio_control_ok');
const s3OK = new Rate('maxio_s3_ok');
const processingExpectationConfigOK = new Rate('maxio_processing_expectation_config_ok');
const s3Objects = new Counter('maxio_s3_objects');

function boolEnv(value) {
  return parseBoolExpectation(value) === true;
}

function parseBoolExpectation(value) {
  const normalized = String(value || '').trim().toLowerCase();
  if (['1', 'true', 'yes', 'on'].includes(normalized)) {
    return true;
  }
  if (['0', 'false', 'no', 'off'].includes(normalized)) {
    return false;
  }
  return null;
}

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

function versionIDFromResponse(response) {
  return response.headers['X-Amz-Version-Id'] || response.headers['x-amz-version-id'] || '';
}

function processingRecordURL(bucket, key, versionID) {
  return `${baseURL}/_processing/records?bucket=${encodeURIComponent(bucket)}&key=${encodeURIComponent(key)}&version_id=${encodeURIComponent(versionID)}`;
}

function processingDigestRecordURL(bucket, key, digest) {
  return `${baseURL}/_processing/records?bucket=${encodeURIComponent(bucket)}&key=${encodeURIComponent(key)}&digest=${encodeURIComponent(digest)}`;
}

function processingRecordListURL(status) {
  return `${baseURL}/_processing/records?status=${encodeURIComponent(status)}&limit=${processingRecordListLimit}`;
}

function getProcessingRecord(bucket, key, versionID) {
  let response = null;
  for (let attempt = 0; attempt <= processingRecordRetries; attempt += 1) {
    response = http.get(processingRecordURL(bucket, key, versionID), adminHeaders());
    if (response.status === 200 && processingRecordReadyForReadResponse(response)) {
      return response;
    }
    if (attempt === processingRecordRetries) {
      return response;
    }
    sleep(processingRecordRetrySleep);
  }
  return response;
}

function processingRecordReadyForReadResponse(response) {
  if (response.status !== 200) {
    return false;
  }
  const payload = response.json();
  if (!processingRecordHasExpectedResultMetadata(payload)) {
    return false;
  }
  return payload.read_allowed !== false;
}
function getProcessingDigestRecord(bucket, key, digest) {
  let response = null;
  for (let attempt = 0; attempt <= processingRecordRetries; attempt += 1) {
    response = http.get(processingDigestRecordURL(bucket, key, digest), adminHeaders());
    if (response.status === 200) {
      return response;
    }
    if (attempt === processingRecordRetries) {
      return response;
    }
    sleep(processingRecordRetrySleep);
  }
  return response;
}

function checkProcessingExpectationConfig() {
  processingExpectationConfigOK.add(invalidProcessingExpectationCount === 0);
}

function checkProcessingCapabilities(response) {
  if (expectedProcessingCapabilities.length === 0) {
    return;
  }
  check(response, {
    'processing status has expected capabilities': (r) => {
      if (r.status !== 200) {
        return false;
      }
      const payload = r.json();
      const capabilities = Array.isArray(payload.capabilities) ? payload.capabilities : [];
      return expectedProcessingCapabilities.every((capability) => capabilities.includes(capability));
    },
  });
}

function checkProcessingProcessors(response) {
  if (expectedProcessingProcessors.length === 0) {
    return;
  }
  check(response, {
    'processing status has expected processors': (r) => {
      if (r.status !== 200) {
        return false;
      }
      const payload = r.json();
      const processors = Array.isArray(payload.processors) ? payload.processors : [];
      return expectedProcessingProcessors.every((processor) => processors.includes(processor));
    },
  });
}

function checkProcessingProcessorModes(response) {
  if (expectedProcessingProcessorModes.length === 0) {
    return;
  }
  check(response, {
    'processing status has expected processor modes': (r) => {
      if (r.status !== 200) {
        return false;
      }
      const payload = r.json();
      const processorModes = payload.processor_modes && typeof payload.processor_modes === 'object' ? payload.processor_modes : {};
      return expectedProcessingProcessorModes.every(({ processor, mode, valid }) => valid && processorModes[processor] === mode);
    },
  });
}

function checkProcessingProcessorFailOpen(response) {
  if (expectedProcessingProcessorFailOpen.length === 0) {
    return;
  }
  check(response, {
    'processing status has expected processor fail-open': (r) => {
      if (r.status !== 200) {
        return false;
      }
      const payload = r.json();
      const processorFailOpen = payload.processor_fail_open && typeof payload.processor_fail_open === 'object' ? payload.processor_fail_open : {};
      return expectedProcessingProcessorFailOpen.every(({ processor, failOpen, valid }) => valid && processorFailOpen[processor] === failOpen);
    },
  });
}

function processingRecordHasExpectedResultMetadataResponse(response) {
  if (expectedProcessingResultMetadata.length === 0 || response.status !== 200) {
    return true;
  }
  return processingRecordHasExpectedResultMetadata(response.json());
}

function processingRecordHasExpectedResultMetadata(payload) {
  const results = Array.isArray(payload.results) ? payload.results : [];
  return expectedProcessingResultMetadata.every(({ processor, key, expected, valid }) => {
    if (!valid) {
      return false;
    }
    const result = results.find((item) => item && item.processor === processor);
    if (!result || !result.metadata || typeof result.metadata !== 'object') {
      return false;
    }
    const actual = result.metadata[key];
    if (actual === undefined || actual === null || actual === '') {
      return false;
    }
    return expected === null || String(actual) === expected;
  });
}
function checkProcessingRecordList() {
  if (processingRecordListStatus === '') {
    return;
  }
  const response = http.get(processingRecordListURL(processingRecordListStatus), adminHeaders());
  check(response, {
    'processing record list is visible': (r) => recordControl(r) && r.status === 200,
    'processing record list has records array': (r) => {
      if (r.status !== 200) {
        return false;
      }
      const payload = r.json();
      return Array.isArray(payload.records);
    },
    'processing record list status matches filter': (r) => {
      if (r.status !== 200) {
        return false;
      }
      const payload = r.json();
      const records = Array.isArray(payload.records) ? payload.records : [];
      return records.every((record) => record.status === processingRecordListStatus);
    },
  });
}

function checkProcessingRecord(bucket, key, put) {
  if (!processingRecordCheck) {
    return;
  }
  const versionID = versionIDFromResponse(put);
  if (versionID === '') {
    check(put, { 'processing record has version id': () => false });
    return;
  }
  const record = getProcessingRecord(bucket, key, versionID);
  check(record, {
    'processing record is visible': (r) => recordControl(r) && r.status === 200,
    'processing record status is terminal or active': (r) => {
      if (r.status !== 200) {
        return false;
      }
      const payload = r.json();
      return ['queued', 'running', 'succeeded', 'skipped'].includes(payload.status);
    },
    'processing record exposes read decision': (r) => {
      if (r.status !== 200) {
        return false;
      }
      const payload = r.json();
      return typeof payload.read_allowed === 'boolean';
    },
    'processing record has expected result metadata': (r) => {
      if (r.status !== 200) {
        return false;
      }
      return processingRecordHasExpectedResultMetadata(r.json());
    },
  });
}

export default function () {
  group('control-plane', () => {
    check(http.get(`${baseURL}/healthz`), { 'healthz is 200': (r) => recordControl(r) && r.status === 200 });
    check(http.get(`${baseURL}/readyz`), { 'readyz is 200': (r) => recordControl(r) && r.status === 200 });
    check(http.get(`${baseURL}/metrics`, adminHeaders()), { 'metrics is 200': (r) => recordControl(r) && r.status === 200 });
    check(http.get(`${baseURL}/_s3/upstreams`, adminHeaders()), { 'upstreams is 200': (r) => recordControl(r) && r.status === 200 });
    check(http.get(`${baseURL}/_index/status`, adminHeaders()), { 'index status is 200': (r) => recordControl(r) && r.status === 200 });
    const processingStatus = http.get(`${baseURL}/_processing/status`, adminHeaders());
    check(processingStatus, { 'processing status is 200': (r) => recordControl(r) && r.status === 200 });
    checkProcessingExpectationConfig();
    checkProcessingCapabilities(processingStatus);
    checkProcessingProcessors(processingStatus);
    checkProcessingProcessorModes(processingStatus);
    checkProcessingProcessorFailOpen(processingStatus);
    checkProcessingRecordList();
  });

  group('s3-proxy', () => {
    const bucket = selectBucket();
    const key = `k6/${exec.vu.idInTest}/${exec.scenario.iterationInTest}.txt`;
    const url = `${s3URL}/${bucket}/${key}`;
    const body = objectBody();
    const put = http.put(url, body, { headers: { 'Content-Type': 'text/plain' } });
    check(put, { 'put object accepted': (r) => recordS3(r) && [200, 201, 204].includes(r.status) });
    checkProcessingRecord(bucket, key, put);

    const get = http.get(url);
    check(get, { 'get object accepted': (r) => recordS3(r) && r.status === 200 });

    const del = http.del(url);
    check(del, { 'delete object accepted': (r) => recordS3(r) && [200, 202, 204, 404].includes(r.status) });
    s3Objects.add(1);
  });

  if (clamavBlockCheck) {
    group('clamav-block', () => {
      const bucket = selectBucket();
      const key = `k6/eicar/${exec.vu.idInTest}/${exec.scenario.iterationInTest}.txt`;
      const put = http.put(`${s3URL}/${bucket}/${key}`, eicarBody, { headers: { 'Content-Type': 'text/plain' } });
      check(put, { 'clamav blocks eicar': (r) => [403, 423].includes(r.status) });
      const record = getProcessingDigestRecord(bucket, key, eicarDigest);
      check(record, {
        'clamav blocked record is visible': (r) => recordControl(r) && r.status === 200,
        'clamav blocked record status is blocked': (r) => {
          if (r.status !== 200) {
            return false;
          }
          const payload = r.json();
          return payload.status === 'blocked' && payload.read_allowed === false;
        },
        'clamav blocked record has infected verdict': (r) => {
          if (r.status !== 200) {
            return false;
          }
          const payload = r.json();
          const results = Array.isArray(payload.results) ? payload.results : [];
          const result = results.find((item) => item && item.processor === 'clamav');
          return result && result.metadata && result.metadata.verdict === 'infected' && Boolean(result.metadata.signature) && Boolean(result.metadata.response);
        },
      });
    });
  }

  sleep(1);
}





