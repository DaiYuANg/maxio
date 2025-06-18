use crate::s3::bucket_handler::{
    create_bucket, delete_bucket, get_bucket_acl, get_bucket_cors, get_bucket_location,
    get_bucket_policy, list_buckets, put_bucket_acl, put_bucket_cors, put_bucket_policy,
};
use crate::s3::object_handler::{delete_object, get_object, head_object, list_objects, put_object};
use crate::s3::openapi::ApiDoc;
use crate::state::AppState;
use axum::routing::{delete, get, head, put};
use axum::Router;
use axum_prometheus::PrometheusMetricLayer;
use tower_http::trace::TraceLayer;
use tracing::debug;
use utoipa::OpenApi;
use utoipa_swagger_ui::SwaggerUi;

pub struct S3Server {
  router: Router,
  address: String,
}

impl S3Server {
  pub fn new(address: String) -> Self {
    let (prom_layer, metric_handle) = PrometheusMetricLayer::pair();
    // build our application with a route
    let app = Router::new()
      .route("/", get(list_buckets))
      // bucket 操作
      .route("/{bucket}", put(create_bucket))
      .route("/{bucket}", delete(delete_bucket))
     
      .route("/{bucket}", get(list_objects))
      .route("/{bucket}/{key}", put(put_object))
      .route("/{bucket}/{key}", get(get_object))
      .route("/{bucket}/{key}", head(head_object))
      .route("/{bucket}/{key}", delete(delete_object))
      .route(
        "/metrics",
        get(move || async move { metric_handle.render() }),
      )
      .merge(SwaggerUi::new("/swagger-ui").url("/api-doc/openapi.json", ApiDoc::openapi()))
      .layer(prom_layer)
      .layer(TraceLayer::new_for_http())
      .with_state(AppState {});
    S3Server {
      router: app,
      address,
    }
  }

  pub async fn start(&self) {
    // run our app with hyper, listening globally on port 3000
    debug!("Starting S3 Server:http://{}", self.address);
    debug!("Starting S3 Server:http://{}/swagger-ui", self.address);
    let listener = tokio::net::TcpListener::bind(self.address.clone())
      .await
      .unwrap();
    axum::serve(listener, self.router.clone()).await.unwrap();
  }
}
