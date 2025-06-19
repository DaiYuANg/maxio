use serde::{Deserialize, Serialize};
use std::net::{IpAddr, SocketAddr};
use std::path::PathBuf;
use std::time::Duration;

/// 存储引擎类型
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
pub enum StorageEngine {
  /// 简单文件系统存储（开发用）
  SimpleFs,
  /// 日志结构合并存储（生产推荐）
  Lsm,
  /// 分片存储引擎
  Sharded,
}

/// 纠删码配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErasureCodingConfig {
  /// 数据分片数量
  pub data_shards: usize,
  /// 校验分片数量
  pub parity_shards: usize,
  /// 分片大小阈值（小于此值不分片）
  pub min_shard_size: u64,
  /// 最大分片大小
  pub max_shard_size: u64,
}

/// 事务配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TransactionConfig {
  /// 事务超时时间
  pub timeout: Duration,
  /// 是否启用两阶段提交
  pub enable_2pc: bool,
  /// WAL 日志目录
  pub wal_path: PathBuf,
  /// WAL 文件最大大小
  pub max_wal_size: u64,
  /// 事务隔离级别
  pub isolation_level: IsolationLevel,
}

/// 事务隔离级别
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
pub enum IsolationLevel {
  ReadUncommitted,
  ReadCommitted,
  RepeatableRead,
  Serializable,
}

/// 缓存配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CacheConfig {
  /// 元数据缓存大小（字节）
  pub metadata_cache_size: usize,
  /// 数据块缓存大小（字节）
  pub data_block_cache_size: usize,
  /// 小文件合并缓冲区大小
  pub merge_buffer_size: usize,
  /// 合并操作超时时间
  pub merge_timeout: Duration,
}

/// 集群配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ClusterConfig {
  /// 当前节点ID
  pub node_id: String,
  /// 集群成员地址列表
  pub members: Vec<SocketAddr>,
  /// 协调节点地址
  pub coordinator: Option<SocketAddr>,
  /// 节点发现间隔
  pub discovery_interval: Duration,
  /// 心跳超时时间
  pub heartbeat_timeout: Duration,
}

/// 安全配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SecurityConfig {
  /// 是否启用TLS
  pub enable_tls: bool,
  /// TLS证书路径
  pub cert_path: Option<PathBuf>,
  /// TLS私钥路径
  pub key_path: Option<PathBuf>,
  /// 访问密钥（Access Key）
  pub access_key: Option<String>,
  /// 秘密密钥（Secret Key）
  pub secret_key: Option<String>,
  /// 是否启用匿名访问
  pub allow_anonymous: bool,
}

/// 性能调优配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PerformanceConfig {
  /// IO线程数
  pub io_threads: usize,
  /// 工作线程数
  pub worker_threads: usize,
  /// 最大并发连接数
  pub max_connections: usize,
  /// 最大请求体大小（字节）
  pub max_body_size: usize,
  /// 是否启用io_uring（Linux专属）
  pub enable_io_uring: bool,
  /// NUMA节点亲和性（可选）
  pub numa_node: Option<usize>,
}

/// 日志配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogConfig {
  /// 日志级别
  pub level: String,
  /// 日志文件路径（None表示输出到控制台）
  pub file_path: Option<PathBuf>,
  /// 日志文件最大大小（字节）
  pub max_file_size: u64,
  /// 保留的日志文件数量
  pub max_files: usize,
}

/// 监控配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MonitoringConfig {
  /// Prometheus指标端口
  pub metrics_port: u16,
  /// 健康检查端点
  pub health_check_endpoint: String,
  /// 性能采样间隔
  pub profiling_interval: Duration,
}

/// 搜索服务配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SearchConfig {
  /// 是否启用内容搜索
  pub enable_content_search: bool,
  /// 全文索引引擎路径
  pub index_path: PathBuf,
  /// 索引刷新间隔
  pub index_refresh_interval: Duration,
  /// 支持的文件类型扩展名
  pub supported_extensions: Vec<String>,
}

/// 主服务配置结构体
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ServiceConfig {
  /// 服务名称
  pub service_name: String,
  /// 服务绑定地址
  pub bind_address: IpAddr,
  /// HTTP服务端口
  pub http_port: u16,
  /// RPC服务端口（集群内部通信）
  pub rpc_port: u16,
  /// 数据存储根目录
  pub data_root: PathBuf,
  /// 临时文件目录
  pub temp_dir: PathBuf,
  /// 存储引擎类型
  pub storage_engine: StorageEngine,
  /// 纠删码配置
  pub erasure_coding: ErasureCodingConfig,
  /// 事务配置
  pub transaction: TransactionConfig,
  /// 缓存配置
  pub cache: CacheConfig,
  /// 集群配置
  pub cluster: ClusterConfig,
  /// 安全配置
  pub security: SecurityConfig,
  /// 性能配置
  pub performance: PerformanceConfig,
  /// 日志配置
  pub log: LogConfig,
  /// 监控配置
  pub monitoring: MonitoringConfig,
  /// 搜索配置
  pub search: SearchConfig,
}
