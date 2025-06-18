use crate::s3::bucket_handler::__path_create_bucket;
use crate::s3::bucket_handler::__path_delete_bucket;
use crate::s3::bucket_handler::__path_list_buckets;
use crate::s3::object_handler::__path_delete_object;
use crate::s3::object_handler::__path_get_object;
use crate::s3::object_handler::__path_head_object;
use crate::s3::object_handler::__path_list_objects;
use crate::s3::object_handler::__path_put_object;
use utoipa::OpenApi;
pub const S3_TAG: &str = "s3";

#[derive(OpenApi)]
#[openapi(tags(
        (name = S3_TAG, description = "S3 compatible"),
), paths(
        delete_object,
        head_object,
        get_object,
        list_objects,
        put_object,
        list_buckets,
        delete_bucket,
        create_bucket
        )
)
]
pub struct ApiDoc;
