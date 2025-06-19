use std::collections::BTreeMap;
use std::time::Instant;

// 在内存中聚合多个小文件为一个逻辑块
struct ObjectGroup {
    buffer: Vec<u8>,          // 合并后的数据块（如 64MB）
    meta_index: BTreeMap<String, (u64, u64)>, // 文件名 -> (offset, length)
    created_at: Instant,
}

impl ObjectGroup {
    // 添加小文件到组
    pub fn add_file(&mut self, name: &str, data: &[u8]) {
        let offset = self.buffer.len();
        self.buffer.extend_from_slice(data);
        self.meta_index.insert(name.to_string(), (offset as u64, data.len() as u64));
    }

    // // 达到阈值后持久化（触发纠删码编码）
    // pub fn flush_to_disk(&self, backend: &StorageBackend) {
    //     let group_id = generate_uuid();
    //     backend.write_merged(&group_id, &self.buffer); // 大块写入
    //     backend.write_index(&group_id, &self.meta_index); // 元数据单独存储
    // }
}