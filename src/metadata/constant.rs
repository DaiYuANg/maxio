use bincode::{Decode, Encode};

#[derive(serde::Serialize, serde::Deserialize, Debug, Clone, Decode, Encode)]
pub enum Effect {
  Allow,
  Deny,
}

#[derive(serde::Serialize, serde::Deserialize, Debug, Clone, Decode, Encode)]
pub enum Action {
  GetObject,
  PutObject,
  DeleteObject,
  ListBucket,
}
