# S3 compatibility

MaxIO exposes an S3-compatible HTTP surface under the `/s3` path prefix. The
compatibility target is the stable subset needed by common SDK object workflows,
not full AWS S3 parity.

## Endpoint and signing

- Use path-style addressing: `http://host:port/s3/<bucket>/<key>`.
- Configure SDK clients with the same region as `S3Region`; MaxIO defaults to
  `us-east-1` when no region is set.
- Signature V4 header auth and presigned URLs are supported when
  `S3AccessKey` and `S3SecretKey` are configured.
- Presigned URLs accept a maximum lifetime of 7 days and use a 15 minute clock
  skew window.
- If no S3 credentials are configured, the compatibility endpoint accepts
  unsigned requests.

## Supported object and bucket operations

- Bucket operations: create bucket, delete bucket, head bucket, list buckets,
  get bucket location, list objects V1, and list objects V2.
- ListObjectsV2 supports `prefix`, `delimiter`, `max-keys`,
  `continuation-token`, and `start-after`.
- Object operations: put object, head object, get object, delete object, copy
  object, and multi-object delete.
- Object metadata headers round-trip for `Content-Type`, `Cache-Control`,
  `Content-Disposition`, `Content-Encoding`, `Content-Language`, and
  `x-amz-meta-*`.
- GET supports a single HTTP byte range, including bounded, open-ended, and
  suffix ranges. Multi-range requests are rejected with `InvalidRange`.
- Error responses are XML `Error` documents with S3-style error codes such as
  `NoSuchBucket`, `NoSuchKey`, `InvalidRange`, and `AccessDenied`.

## Multipart uploads

- Supported operations: initiate multipart upload, upload part, list parts,
  complete multipart upload, abort multipart upload, and list multipart uploads.
- Complete multipart upload enforces S3's minimum size rule for non-final parts:
  every non-final part must be at least 5 MiB.
- List parts and list multipart uploads support pagination and prefix filtering
  for the currently implemented subset.

## ETag behavior

- ETags are quoted and stable across `PUT`, `HEAD`, `GET`, and list responses.
- Single-part object ETags are derived from MaxIO's content hash, not AWS S3's
  MD5 convention. Do not use the ETag as an MD5 checksum.
- Multipart complete responses return a quoted multipart-style ETag with the
  `-<part-count>` suffix.

## Known gaps

- Virtual-hosted-style bucket addressing is not supported; use path-style
  addressing with the `/s3` prefix.
- ACLs, bucket policies, versioning, object tagging, lifecycle rules,
  server-side encryption controls, and storage class transitions are not part of
  the current compatibility subset.
- Conditional requests such as `If-Match`, `If-None-Match`, and date-based
  preconditions are not implemented.
- AWS-specific response override query parameters may be accepted for signing
  compatibility but are not applied to response headers.

## SDK notes

- Set the SDK endpoint to include `/s3`.
- Force path-style addressing.
- Use Signature V4.
- Do not enable SDK features that require ACLs, versioning, tagging, or bucket
  policy APIs.
- Treat ETags as opaque validators, not MD5 checksums.
