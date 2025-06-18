use crate::metadata::config::BucketConfig;
use crate::metadata::policy::BucketPolicy;
use bincode::{Decode, Encode};
use redb::{TypeName, Value};
use std::borrow::Cow;
use crate::impl_redb_value;

#[derive(serde::Serialize, serde::Deserialize, Debug, Clone, Encode, Decode)]
pub struct BucketMeta {
  pub id: String,                   // 内部唯一标识
  pub name: String,                 // Bucket 名称，用户可见
  pub created_at: i64,              // 创建时间
  pub owner: String,                // 所有者（可选）
  pub policy: Option<BucketPolicy>, // 权限策略
  pub config: BucketConfig,         // 存储策略等配置
}

impl_redb_value!(BucketMeta, "BucketMeta");

// impl Value for BucketMeta {
//   type SelfType<'a>
//     = BucketMeta
//   where
//     Self: 'a;
//   type AsBytes<'a>
//     = Cow<'a, [u8]>
//   where
//     Self: 'a;
// 
//   fn fixed_width() -> Option<usize> {
//     None // 不定长结构，返回 None
//   }
// 
//   fn from_bytes<'a>(data: &'a [u8]) -> Self::SelfType<'a>
//   where
//     Self: 'a,
//   {
//     bincode::decode_from_slice(data, bincode::config::standard())
//       .map(|(val, _len)| val)
//       .expect("bincode decode failed")
//   }
// 
//   fn as_bytes<'a, 'b: 'a>(value: &'a Self::SelfType<'b>) -> Self::AsBytes<'a> {
//     let mut buffer = Vec::with_capacity(128); // 初始 buffer 大小，视实际结构体调整
//     bincode::encode_into_slice(value, &mut buffer, bincode::config::standard())
//       .expect("bincode encode failed");
//     Cow::Owned(buffer)
//   }
// 
//   fn type_name() -> TypeName {
//     TypeName::new("BucketMeta")
//   }
// }
