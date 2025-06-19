use crate::max::{MaxServer, PutRequest};
use tokio::net::TcpListener;
use tracing::debug;

pub async fn handle_put(req: PutRequest) {
  println!(
    "[PUT] key: {}, value: {:?}, ttl: {:?}",
    req.key, req.value, req.ttl
  );
  // TODO: Replace with actual storage logic
}

impl MaxServer {
  pub fn new(addr: &str) -> Self {
    Self {
      address: addr.to_string(),
    }
  }

  pub async fn start(&self) {
    let listener = TcpListener::bind(&self.address).await.unwrap();
    debug!("Max Server listening on {}", self.address);

    loop {
      let (stream, addr) = listener.accept().await.unwrap();
      println!("Accepted connection from {}", addr);
      tokio::spawn(async move {});
    }
  }
}
