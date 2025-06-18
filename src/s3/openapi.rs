use crate::s3::S3_TAG;
use crate::s3::__path_delete_object;
use crate::s3::__path_get_object;
use crate::s3::__path_head_object;
use utoipa::OpenApi;

#[derive(OpenApi)]
#[openapi(tags(
        (name = S3_TAG, description = "S3 compatible"),
), paths(delete_object, head_object, get_object))]
pub struct ApiDoc;
