mod server;

use serde::{Deserialize, Serialize};

#[derive(Clone)]
pub struct MaxServer {
    address: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PutRequest {
    pub key: String,
    pub value: Vec<u8>,
    pub ttl: Option<u64>,
}