use axum::{
    body::Body,
    extract::Path,
    http::{HeaderMap, StatusCode},
    response::Response,
    Json,
};
use std::collections::HashMap;
use tracing::debug;

pub const BUCKET_TAG: &str = "bucket";
// List Buckets - GET /
#[utoipa::path(
    get,
    path = "/",
    tag = BUCKET_TAG,
    responses(
        (status = 200, description = "Object uploaded successfully"),
        (status = 500, description = "Internal server error")
    )
)]
pub async fn list_buckets() -> Response {
  // TODO: 返回符合 S3 规范的 XML
  Response::builder()
    .status(StatusCode::OK)
    .header("Content-Type", "application/xml")
    .body(Body::from(
      "<ListAllMyBucketsResult>...</ListAllMyBucketsResult>",
    ))
    .unwrap()
}

// Create Bucket - PUT /{bucket}
/// create bucket
#[utoipa::path(
    put,
    path = "/{bucket}",
    params(
        ("bucket" = String, Path, description = "Bucket 名称")
    ),
    responses(
        (status = 200, description = "Bucket created"),
        (status = 400, description = "Invalid bucket name"),
    ),
    tag = BUCKET_TAG
)]
pub async fn create_bucket(Path(bucket): Path<String>) -> StatusCode {
  debug!("Create bucket: {}", bucket);
  StatusCode::NO_CONTENT // 204
}

// Delete Bucket - DELETE /{bucket}
/// 删除 bucket
#[utoipa::path(
    delete,
    path = "/{bucket}",
    params(
        ("bucket" = String, Path, description = "Bucket 名称")
    ),
    responses(
        (status = 200, description = "Bucket deleted"),
        (status = 404, description = "Bucket not found"),
    ),
    tag = BUCKET_TAG
)]
pub async fn delete_bucket(Path(bucket): Path<String>) -> StatusCode {
  println!("Delete bucket: {}", bucket);
  StatusCode::NO_CONTENT
}

// Get Bucket Policy - GET /{bucket}?policy
pub async fn get_bucket_policy(Path(bucket): Path<String>) -> Response {
  // TODO: 返回 JSON 格式的策略，header Content-Type: application/json
  Response::builder()
    .status(StatusCode::OK)
    .header("Content-Type", "application/json")
    .body(Body::from(r#"{"Version":"2012-10-17","Statement":[]}"#))
    .unwrap()
}

// Put Bucket Policy - PUT /{bucket}?policy
pub async fn put_bucket_policy(
  Path(bucket): Path<String>,
  policy: Json<HashMap<String, serde_json::Value>>,
) -> StatusCode {
  println!("Put bucket policy for {}: {:?}", bucket, policy);
  StatusCode::NO_CONTENT
}

// Get Bucket Location - GET /{bucket}?location
pub async fn get_bucket_location(Path(bucket): Path<String>) -> Response {
  let xml = format!(
    r#"<?xml version="1.0" encoding="UTF-8"?>
        <LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>"#
  );

  Response::builder()
    .status(StatusCode::OK)
    .header("Content-Type", "application/xml")
    .body(Body::from(xml))
    .unwrap()
}

// Get Bucket ACL - GET /{bucket}?acl
pub async fn get_bucket_acl(Path(bucket): Path<String>) -> Response {
  let xml = r#"<?xml version="1.0" encoding="UTF-8"?>
    <AccessControlPolicy xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
      <Owner>
        <ID>owner-id</ID>
        <DisplayName>owner-name</DisplayName>
      </Owner>
      <AccessControlList>
        <Grant>
          <Grantee xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="CanonicalUser">
            <ID>grantee-id</ID>
            <DisplayName>grantee-name</DisplayName>
          </Grantee>
          <Permission>FULL_CONTROL</Permission>
        </Grant>
      </AccessControlList>
    </AccessControlPolicy>"#;

  Response::builder()
    .status(StatusCode::OK)
    .header("Content-Type", "application/xml")
    .body(Body::from(xml))
    .unwrap()
}

// Put Bucket ACL - PUT /{bucket}?acl
pub async fn put_bucket_acl(Path(bucket): Path<String>, _headers: HeaderMap) -> StatusCode {
  println!("Put bucket ACL: {}", bucket);
  StatusCode::NO_CONTENT
}

// Get Bucket CORS - GET /{bucket}?cors
pub async fn get_bucket_cors(Path(bucket): Path<String>) -> Response {
  let xml = r#"<?xml version="1.0" encoding="UTF-8"?>
    <CORSConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
      <CORSRule>
        <AllowedOrigin>*</AllowedOrigin>
        <AllowedMethod>GET</AllowedMethod>
      </CORSRule>
    </CORSConfiguration>"#;

  Response::builder()
    .status(StatusCode::OK)
    .header("Content-Type", "application/xml")
    .body(Body::from(xml))
    .unwrap()
}

// Put Bucket CORS - PUT /{bucket}?cors
pub async fn put_bucket_cors(Path(bucket): Path<String>, _body: Body) -> StatusCode {
  println!("Put bucket CORS: {}", bucket);
  StatusCode::NO_CONTENT
}
