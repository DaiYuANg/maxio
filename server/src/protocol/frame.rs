struct FrameHeader {
    version: u8,            // 协议版本
    frame_type: u8,         // 类型
    flags: u8,              // 压缩/加密等
    reserved: u8,           // 保留
    payload_len: u32,       // 负载长度
}

pub enum Frame {
    DslQuery(String),
    DslResponse(Vec<u8>),
    // FileUploadInit(FileMetadata),
    FileUploadChunk(Vec<u8>),
    FileUploadFinish,
    FileDownloadInit(String),
    FileDownloadChunk(Vec<u8>),
    FileDownloadFinish,
    Heartbeat,
    Error(String),
}
