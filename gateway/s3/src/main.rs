use crate::server::S3Server;
use tracing_subscriber::EnvFilter;

mod auth;
mod bucket_handler;
mod config;
mod object_handler;
mod openapi;
pub mod server;
mod state;

#[tokio::main]
async fn main() {
  tracing_subscriber::fmt()
    .with_env_filter(EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()))
    .with_env_filter(EnvFilter::from_default_env().add_directive("debug".parse().unwrap()))
    .with_ansi(true)
    .with_file(true)
    .with_line_number(true)
    .with_thread_ids(true)
    .with_thread_names(true)
    .with_filter_reloading()
    .init();
  S3Server::new("0.0.0.0:3000".to_string()).start().await
}
