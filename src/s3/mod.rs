mod openapi;
pub mod server;
mod auth;
mod object_handler;
mod bucket_handler;

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


