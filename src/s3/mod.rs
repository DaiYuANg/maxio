mod openapi;
pub mod server;
mod auth;

use crate::state::AppState;
use axum::{
    Json,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
    response::{IntoResponse, Response},
};
use serde::{Deserialize, Serialize};
use tracing::debug;
use utoipa::ToSchema;

const S3_TAG: &str = "s3";

// PUT /{bucket}/{key} 上传对象
#[utoipa::path(
    put,
    path = "/{bucket}/{key}",
    tag = S3_TAG,
    request_body(content = Vec<u8>, description = "Object binary data"),
    params(
        ("bucket" = String, Path, description = "Bucket name"),
        ("key" = String, Path, description = "Object key")
    ),
    responses(
        (status = 200, description = "Object uploaded successfully"),
        (status = 500, description = "Internal server error")
    )
)]
pub async fn put_object(
    State(_state): State<AppState>,
    Path((_bucket, _key)): Path<(String, String)>,
    _body: Bytes,
) -> impl IntoResponse {
    StatusCode::OK
}

// GET /{bucket}/{key} 下载对象
#[utoipa::path(
    get,
    path = "/{bucket}/{key}",
tag = S3_TAG,
    params(
        ("bucket" = String, Path, description = "Bucket name"),
        ("key" = String, Path, description = "Object key")
    ),
    responses(
        (status = 200, description = "Object retrieved successfully", content_type = "application/octet-stream"),
        (status = 404, description = "Object not found")
    )
)]
pub async fn get_object(
    State(_state): State<AppState>,
    Path((_bucket, _key)): Path<(String, String)>,
) -> impl IntoResponse {
    debug!("get_object called for bucket {:?}", _bucket);
    Response::builder()
        .status(StatusCode::OK)
        .header("Content-Type", "application/octet-stream")
        .body(axum::body::Body::from("fake file content"))
        .unwrap()
}

// HEAD /{bucket}/{key} 获取元数据

#[utoipa::path(
    head,
    path = "/{bucket}/{key}",
tag = S3_TAG,
    params(
        ("bucket" = String, Path, description = "Bucket name"),
        ("key" = String, Path, description = "Object key")
    ),
    responses(
        (status = 200, description = "Metadata retrieved successfully", headers(
            ("Content-Length" = String, description = "Length of the object"),
            ("Content-Type" = String, description = "Content type of the object")
        )),
        (status = 404, description = "Object not found")
    )
)]
pub async fn head_object(
    State(_state): State<AppState>,
    Path((_bucket, _key)): Path<(String, String)>,
) -> impl IntoResponse {
    let mut headers = HeaderMap::new();
    headers.insert("Content-Length", "123".parse().unwrap());
    headers.insert("Content-Type", "application/octet-stream".parse().unwrap());

    (StatusCode::OK, headers)
}

// DELETE /{bucket}/{key} 删除对象

#[utoipa::path(
    delete,
    path = "/{bucket}/{key}",
tag = S3_TAG,
    params(
        ("bucket" = String, Path, description = "Bucket name"),
        ("key" = String, Path, description = "Object key")
    ),
    responses(
        (status = 204, description = "Object deleted successfully"),
        (status = 404, description = "Object not found")
    )
)]
pub async fn delete_object(
    State(_state): State<AppState>,
    Path((_bucket, _key)): Path<(String, String)>,
) -> impl IntoResponse {
    StatusCode::NO_CONTENT
}

#[derive(serde::Serialize, ToSchema)]
struct ObjectInfo {
    key: String,
    size: u64,
}

// GET /{bucket} 列出对象
#[utoipa::path(
    get,
    path = "/{bucket}",
    tag = S3_TAG,
    params(
        ("bucket" = String, Path, description = "Bucket name")
    ),
    responses(
        (status = 200, description = "List objects in bucket", body = [ObjectInfo]),
        (status = 404, description = "Bucket not found")
    )
)]
pub async fn list_objects(
    State(_state): State<AppState>,
    Path(_bucket): Path<String>,
) -> impl IntoResponse {
    // 假数据结构，可用作兼容 AWS ListObjectsV1

    let mock_objects = vec![
        ObjectInfo {
            key: "file1.txt".into(),
            size: 1024,
        },
        ObjectInfo {
            key: "image.jpg".into(),
            size: 2048,
        },
    ];

    Json(mock_objects)
}

// basic handler that responds with a static string
async fn root() -> &'static str {
    "Hello, World!"
}

async fn create_user(
    // this argument tells axum to parse the request body
    // as JSON into a `CreateUser` type
    Json(payload): Json<CreateUser>,
) -> (StatusCode, Json<User>) {
    // insert your application logic here
    let user = User {
        id: 1337,
        username: payload.username,
    };

    // this will be converted into a JSON response
    // with a status code of `201 Created`
    (StatusCode::CREATED, Json(user))
}

// the input to our `create_user` handler
#[derive(Deserialize)]
struct CreateUser {
    username: String,
}

// the output to our `create_user` handler
#[derive(Serialize)]
struct User {
    id: u64,
    username: String,
}
