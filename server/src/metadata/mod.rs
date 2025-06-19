mod bucket_meta;
mod config;
mod constant;
mod policy;

use crate::metadata::bucket_meta::BucketMeta;
use crate::metadata::config::BucketConfig;
use rand::distr::Alphanumeric;
use rand::{rng, Rng};
use redb::{Database, TableDefinition};
use uuid::Uuid;

#[macro_export]
macro_rules! impl_redb_value {
  ($ty:ty, $type_name:expr) => {
    impl redb::Value for $ty {
      type SelfType<'a>
        = $ty
      where
        Self: 'a;
      type AsBytes<'a>
        = std::borrow::Cow<'a, [u8]>
      where
        Self: 'a;

      fn fixed_width() -> Option<usize> {
        None
      }

      fn from_bytes<'a>(data: &'a [u8]) -> Self::SelfType<'a>
      where
        Self: 'a,
      {
        bincode::decode_from_slice(data, bincode::config::standard())
          .map(|(v, _)| v)
          .expect("bincode decode failed")
      }

      fn as_bytes<'a, 'b: 'a>(value: &'a Self::SelfType<'b>) -> Self::AsBytes<'a>
      where
        Self: 'b,
      {
        let encoded = bincode::encode_to_vec(value.clone(), bincode::config::standard())
          .expect("bincode encode failed");
        std::borrow::Cow::Owned(encoded)
      }
      fn type_name() -> redb::TypeName {
        redb::TypeName::new($type_name)
      }
    }
  };
}

pub const BUCKET_TABLE: TableDefinition<&str, BucketMeta> = TableDefinition::new("bucket");

fn random_string(len: usize) -> String {
  let rng = rng();
  rng
    .sample_iter(&Alphanumeric)
    .take(len)
    .map(char::from)
    .collect()
}
pub fn insert_bucket(db: &Database) -> anyhow::Result<()> {
  let bucket = BucketMeta {
    id: Uuid::now_v7().to_string(),
    name: random_string(10).to_string(),
    created_at: chrono::Utc::now().timestamp(),
    owner: "".to_string(),
    policy: None,
    config: BucketConfig {
      versioning: false,
      dedup: false,
      lifecycle_days: None,
    },
  };

  let tx = db.begin_write()?;
  {
    let mut table = tx.open_table(BUCKET_TABLE)?;
    table.insert("test", &bucket)?;
  }
  tx.commit()?;
  Ok(())
}

pub fn get_bucket(db: &Database) -> anyhow::Result<()> {
  let tx = db.begin_read()?;
  let table = tx.open_table(BUCKET_TABLE)?;
  if let Some(value) = table.get("test")? {
    println!("Got bucket: {:?}", value.value());
  }
  Ok(())
}
