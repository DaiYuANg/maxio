use bincode::{Decode, Encode};

#[derive(serde::Serialize, serde::Deserialize, Debug, Clone, Decode, Encode)]
pub struct BucketConfig {
  pub versioning: bool,
  pub dedup: bool,
  pub lifecycle_days: Option<u32>, // 自动清理时间
}
