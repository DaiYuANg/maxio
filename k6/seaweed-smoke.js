import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import exec from 'k6/execution';
import crypto from 'k6/crypto';

const baseURL = (__ENV.BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const s3URL = (__ENV.S3_URL || 'http://127.0.0.1:8081').replace(/\/$/, '');
const adminToken = __ENV.ADMIN_TOKEN || 'dev-admin-token';

const expectedUpstreams = parseCSV(__ENV.EXPECTED_UPSTREAMS || 'seaweed-a,seaweed-b,seaweed-c', 'EXPECTED_UPSTREAMS');
const buckets = parseCSV(__ENV.BUCKETS || 'maxio-a,maxio-b,maxio-c', 'BUCKETS');

const STATUS_CONTROL = [200];
const STATUS_CONTROL_OR_404 = [200, 404];
const STATUS_S3_WRITE = [200, 201, 204];
const STATUS_S3_READ = [200];
const STATUS_S3_DELETE = [200, 202, 204, 404];
const STATUS_PROCESSING_RECORD_QUERY = [200, 404];
const STATUS_CLAMAV_PUT = [403, 423];

const objectBytes = parseBoundedIntegerEnv('S3_OBJECT_BYTES', 1024, 1, 10 * 1024 * 1024);
const vus = parseBoundedIntegerEnv('VUS', 4, 1, 2000);
const processingRecordCheck = boolEnv(__ENV.PROCESSING_RECORD_CHECK);
const processingRecordRetries = parseBoundedIntegerEnv('PROCESSING_RECORD_RETRIES', 5, 0, 120);
const processingRecordRetrySleep = parseBoundedFloatEnv('PROCESSING_RECORD_RETRY_SLEEP', 0.2, 0, 30);
const processingRecordListStatus = (__ENV.PROCESSING_RECORD_LIST_STATUS || '').trim();
const processingRecordListLimit = parseBoundedIntegerEnv('PROCESSING_RECORD_LIST_LIMIT', 10, 1, 2000);

const expectedProcessingCapabilities = parseCSVOptional(__ENV.PROCESSING_EXPECT_CAPABILITIES);
const expectedProcessingProcessors = parseCSVOptional(__ENV.PROCESSING_EXPECT_PROCESSORS);
const expectedProcessingProcessorModes = parseProcessorModeExpectation(__ENV.PROCESSING_EXPECT_PROCESSOR_MODES);
const expectedProcessingProcessorFailOpen = parseProcessorFailOpenExpectation(__ENV.PROCESSING_EXPECT_PROCESSOR_FAIL_OPEN);
const expectedProcessingResultMetadata = parseProcessorMetadataExpectation(__ENV.PROCESSING_EXPECT_RESULT_METADATA);
const invalidProcessingExpectationCount =
  expectedProcessingProcessorModes.filter(({ valid }) => !valid).length +
  expectedProcessingProcessorFailOpen.filter(({ valid }) => !valid).length +
  expectedProcessingResultMetadata.filter(({ valid }) => !valid).length;

const clamavBlockCheck = boolEnv(__ENV.CLAMAV_BLOCK_CHECK);
const eicarBody = 'X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*';
const eicarDigest = `sha256:${crypto.sha256(eicarBody, 'hex')}`;

export const options = {
  vus,
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

function parseIntLike(value) {
  return /^[-+]?\d+$/.test(value);
}

function parseFloatLike(value) {
  return /^\d*\.?\d+(?:[eE][-+]?\d+)?$/.test(value);
}

function parseBoundedIntegerEnv(name, defaultValue, minValue, maxValue) {
  const raw = __ENV[name];
  const valueText = raw === undefined || String(raw).trim() === '' ? String(defaultValue) : String(raw).trim();
  if (!parseIntLike(valueText)) {
    throw new Error(`${name} must be an integer, got '${valueText}'`);
  }
  const value = Number.parseInt(valueText, 10);
  if (value < minValue || value > maxValue) {
    throw new Error(`${name} must be between ${minValue} and ${maxValue}, got ${value}`);
  }
  return value;
}

function parseBoundedFloatEnv(name, defaultValue, minValue, maxValue) {
  const raw = __ENV[name];
  const valueText = raw === undefined || String(raw).trim() === '' ? String(defaultValue) : String(raw).trim();
  if (!parseFloatLike(valueText)) {
    throw new Error(`${name} must be numeric, got '${valueText}'`);
  }
  const value = Number.parseFloat(valueText);
  if (!Number.isFinite(value) || value < minValue || value > maxValue) {
    throw new Error(`${name} must be between ${minValue} and ${maxValue}, got ${value}`);
  }
  return value;
}

function parseCSV(value, name) {
  const values = String(value)
    .split(',')
    .map((v) => v.trim())
    .filter(Boolean);
  if (values.length === 0) {
    throw new Error(`${name} must contain at least one value`);
  }
  return [...new Set(values)];
}

function parseCSVOptional(value) {
  return String(value || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseBoolExpectation(value) {
  const normalized = String(value || '').trim().toLowerCase();
  if (["1", "true", "yes", "on"].includes(normalized)) {
    return true;
  }
  if (["0", "false", "no", "off"].includes(normalized)) {
    return false;
  }
  return null;
}

function parseProcessorModeExpectation(raw) {
  return parseCSVOptional(raw).map((value) => {
    const separator = value.indexOf(':');
    if (separator < 0) {
      return { processor: value, mode: '', valid: false };
    }
    const processor = value.slice(0, separator).trim();
    const mode = value.slice(separator + 1).trim();
    return { processor, mode, valid: processor !== '' && mode !== '' };
  });
}

function parseProcessorFailOpenExpectation(raw) {
  return parseCSVOptional(raw).map((value) => {
    const separator = value.indexOf(':');
    if (separator < 0) {
      return { processor: value, failOpen: null, valid: false };
    }
    const processor = value.slice(0, separator).trim();
    const failOpen = parseBoolExpectation(value.slice(separator + 1));
    return { processor, failOpen, valid: processor !== '' && failOpen !== null };
  });
}

function parseProcessorMetadataExpectation(raw) {
  return parseCSVOptional(raw).map((value) => {
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
}

function requestOptions(extraHeaders, expectedStatuses) {
  return {
    ...extraHeaders,
    responseCallback: http.expectedStatuses(...expectedStatuses),
  };
}

function parseUpstreamIds(payload) {
  const source = [];
  if (Array.isArray(payload)) {
    source.push(...payload);
  } else if (payload && Array.isArray(payload.upstreams)) {
    source.push(...payload.upstreams);
  } else if (payload && Array.isArray(payload.data)) {
    source.push(...payload.data);
  }

  const ids = new Set();
  for (const item of source) {
    if (typeof item === 'string') {
      const value = item.trim();
      if (value !== '') {
        ids.add(value);
      }
      continue;
    }
    if (!item || typeof item !== 'object') {
      continue;
    }
    if (typeof item.id === 'string' && item.id.trim() !== '') {
      ids.add(item.id.trim());
    }
    if (typeof item.name === 'string' && item.name.trim() !== '') {
      ids.add(item.name.trim());
    }
  }
  return [...ids];
}

function checkExpectedUpstreams(response) {
  if (response.status !== 200) {
    return false;
  }
  const payload = response.json();
  const ids = parseUpstreamIds(payload);
  return expectedUpstreams.every((expectedUpstream) => ids.includes(expectedUpstream));
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

function recordControl(response, expectedStatuses = STATUS_CONTROL) {
  const ok = expectedStatuses.includes(response.status);
  controlOK.add(ok);
  return ok;
}

function recordS3(response, expectedStatuses = STATUS_S3_WRITE) {
  const ok = expectedStatuses.includes(response.status);
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
    response = http.get(processingRecordURL(bucket, key, versionID), requestOptions(adminHeaders(), STATUS_PROCESSING_RECORD_QUERY));
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
    response = http.get(processingDigestRecordURL(bucket, key, digest), requestOptions(adminHeaders(), STATUS_PROCESSING_RECORD_QUERY));
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
  const response = http.get(processingRecordListURL(processingRecordListStatus), requestOptions(adminHeaders(), STATUS_PROCESSING_RECORD_QUERY));
  check(response, {
    'processing record list is visible': (r) => recordControl(r, [200]) && r.status === 200,
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
    'processing record is visible': (r) => recordControl(r, [200]) && r.status === 200,
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
    check(http.get(`${baseURL}/healthz`, requestOptions({}, STATUS_CONTROL)), {
      'healthz is 200': (r) => recordControl(r, STATUS_CONTROL) && r.status === 200,
    });
    check(http.get(`${baseURL}/readyz`, requestOptions({}, STATUS_CONTROL)), {
      'readyz is 200': (r) => recordControl(r, STATUS_CONTROL) && r.status === 200,
    });
    check(http.get(`${baseURL}/metrics`, requestOptions(adminHeaders(), STATUS_CONTROL)), {
      'metrics is 200': (r) => recordControl(r, STATUS_CONTROL) && r.status === 200,
    });

    const upstreams = http.get(`${baseURL}/_s3/upstreams`, requestOptions(adminHeaders(), STATUS_CONTROL));
    check(upstreams, {
      'upstreams is 200': (r) => recordControl(r, STATUS_CONTROL) && r.status === 200,
      'upstreams contains expected entries': (r) => checkExpectedUpstreams(r),
    });

    check(http.get(`${baseURL}/_index/status`, requestOptions(adminHeaders(), STATUS_CONTROL)), {
      'index status is 200': (r) => recordControl(r, STATUS_CONTROL) && r.status === 200,
    });

    const processingStatus = http.get(`${baseURL}/_processing/status`, requestOptions(adminHeaders(), STATUS_CONTROL_OR_404));
    check(processingStatus, {
      'processing status is 200 or 404': (r) => recordControl(r, STATUS_CONTROL_OR_404) && [200, 404].includes(r.status),
    });

    if (processingStatus.status === 200) {
      checkProcessingExpectationConfig();
      checkProcessingCapabilities(processingStatus);
      checkProcessingProcessors(processingStatus);
      checkProcessingProcessorModes(processingStatus);
      checkProcessingProcessorFailOpen(processingStatus);
      checkProcessingRecordList();
    }
  });

  group('s3-proxy', () => {
    const bucket = selectBucket();
    const key = `k6/${exec.vu.idInTest}/${exec.scenario.iterationInTest}.txt`;
    const url = `${s3URL}/${bucket}/${key}`;
    const body = objectBody();

    const put = http.put(url, body, requestOptions({ headers: { 'Content-Type': 'text/plain' } }, STATUS_S3_WRITE));
    check(put, {
      'put object accepted': (r) => recordS3(r, STATUS_S3_WRITE),
    });
    checkProcessingRecord(bucket, key, put);

    const get = http.get(url, requestOptions({}, STATUS_S3_READ));
    check(get, {
      'get object accepted': (r) => recordS3(r, STATUS_S3_READ),
    });

    const del = http.del(url, null, requestOptions({}, STATUS_S3_DELETE));
    check(del, {
      'delete object accepted': (r) => recordS3(r, STATUS_S3_DELETE),
    });

    s3Objects.add(1);
  });

  if (clamavBlockCheck) {
    group('clamav-block', () => {
      const bucket = selectBucket();
      const key = `k6/eicar/${exec.vu.idInTest}/${exec.scenario.iterationInTest}.txt`;
      const put = http.put(
        `${s3URL}/${bucket}/${key}`,
        eicarBody,
        requestOptions({ headers: { 'Content-Type': 'text/plain' } }, STATUS_CLAMAV_PUT),
      );
      check(put, {
        'clamav blocks eicar': (r) => STATUS_CLAMAV_PUT.includes(r.status),
      });
      const record = getProcessingDigestRecord(bucket, key, eicarDigest);
      check(record, {
        'clamav blocked record is visible': (r) => recordControl(r, [200]) && r.status === 200,
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
