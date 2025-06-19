use crate::metadata::constant::{Action, Effect};
use bincode::{Decode, Encode};

#[derive(serde::Serialize, serde::Deserialize, Debug, Clone, Decode, Encode)]
pub struct BucketPolicy {
  pub effect: Effect,          // "Allow" or "Deny"
  pub actions: Vec<Action>,    // 允许的操作，比如 GetObject、PutObject
  pub resources: Vec<String>,  // 作用资源，比如 bucket/object 前缀
  pub principals: Vec<String>, // 可以是用户 ID、角色 ID，或 "*" 表示匿名
}
