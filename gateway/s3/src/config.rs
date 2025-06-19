use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize, Debug, Default)]
struct S3GatewayConfig {
  port: u16,
  address: String,
}

fn load_config() -> S3GatewayConfig {
  return S3GatewayConfig::default();
}
