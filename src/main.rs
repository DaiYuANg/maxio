use crate::s3::server::S3Server;
use tokio::signal;
use tracing_subscriber::EnvFilter;
use crate::max::MaxServer;

mod s3;
mod state;
mod max;
mod bucket;
mod metadata;
mod config;
mod writer;

#[tokio::main]
async fn main() {
    // initialize tracing
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()))
        .with_env_filter(EnvFilter::from_default_env().add_directive("debug".parse().unwrap()))
        .with_ansi(true)
        .init();
    let kv_server = MaxServer::new("127.0.0.1:7000");
    let s3_server = S3Server::new("0.0.0.0:3000".to_string());
    tokio::select! {
        _ = kv_server.start() => {},
        _ = s3_server.start() => {},
        _ = signal::ctrl_c() => {
            println!("Shutdown signal received.");
        }
    }
}
