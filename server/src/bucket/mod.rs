use anyhow::{Result, anyhow};
use redb::{Database, ReadableTable, TableDefinition};
use std::path::Path;
const BUCKETS_META: TableDefinition<&str, &[u8]> = TableDefinition::new("__buckets_meta__");

struct BucketManager {
    db: Database,
}

impl BucketManager {
    pub fn open(path: &Path) -> Result<Self> {
        let db = Database::open(path)?;
        Ok(Self { db })
    }

    pub fn create_bucket(&self, bucket_name: &str) -> Result<()> {
        let write_txn = self.db.begin_write()?; // mutable txn
        {
            let mut meta = write_txn.open_table(BUCKETS_META)?;
            if meta.get(bucket_name)?.is_some() {
                return Err(anyhow!("Bucket already exists"));
            }
            meta.insert(bucket_name, &b"{}"[..])?;
        }

        write_txn.commit()?; // 这里提交
        Ok(())
    }

    pub fn bucket_exists(&self, bucket_name: &str) -> Result<bool> {
        let read_txn = self.db.begin_read()?;
        let meta = read_txn.open_table(BUCKETS_META)?;
        Ok(meta.get(bucket_name)?.is_some())
    }
}
