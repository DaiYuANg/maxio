use crate::s3::openapi::ApiDoc;
use crate::s3::{
  create_user, delete_object, get_object, head_object, list_objects, put_object, root,
};
use crate::state::AppState;
use axum::Router;
use axum::routing::{delete, get, head, post, put};
use axum_prometheus::PrometheusMetricLayer;
use tower_http::trace::TraceLayer;
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
      .route("/", get(root))
      .route("/users", post(create_user))
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
    let listener = tokio::net::TcpListener::bind(self.address.clone())
      .await
      .unwrap();
    axum::serve(listener, self.router.clone()).await.unwrap();
  }
}
