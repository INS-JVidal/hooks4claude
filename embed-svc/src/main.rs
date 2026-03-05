use std::env;
use std::path::PathBuf;
use std::sync::Arc;

use axum::{
    extract::State,
    http::StatusCode,
    routing::{get, post},
    Json, Router,
};
use ort::session::Session;
use ort::value::Tensor;
use serde::{Deserialize, Serialize};
use tokenizers::Tokenizer;
use tokio::signal;
use tokio::sync::Mutex;
use tower_http::cors::CorsLayer;
use tracing::{error, info};

const MAX_BATCH_SIZE: usize = 32;
const DIMENSIONS: usize = 384;
const MODEL_NAME: &str = "all-MiniLM-L6-v2";

struct AppState {
    session: Mutex<Session>,
    tokenizer: Tokenizer,
}

#[derive(Deserialize)]
struct EmbedRequest {
    texts: Vec<String>,
}

#[derive(Serialize)]
struct EmbedResponse {
    embeddings: Vec<Vec<f32>>,
}

#[derive(Serialize)]
struct HealthResponse {
    status: &'static str,
    model: &'static str,
    dimensions: usize,
}

#[derive(Serialize)]
struct ErrorResponse {
    error: String,
}

fn err_500(msg: &str) -> (StatusCode, Json<ErrorResponse>) {
    (
        StatusCode::INTERNAL_SERVER_ERROR,
        Json(ErrorResponse { error: msg.to_string() }),
    )
}

fn l2_normalize(v: &mut [f32]) {
    let norm = v.iter().map(|x| x * x).sum::<f32>().sqrt();
    if norm > 0.0 {
        v.iter_mut().for_each(|x| *x /= norm);
    }
}

async fn embed(
    State(state): State<Arc<AppState>>,
    Json(req): Json<EmbedRequest>,
) -> Result<Json<EmbedResponse>, (StatusCode, Json<ErrorResponse>)> {
    if req.texts.is_empty() {
        return Err((
            StatusCode::BAD_REQUEST,
            Json(ErrorResponse { error: "texts array must not be empty".into() }),
        ));
    }
    if req.texts.len() > MAX_BATCH_SIZE {
        return Err((
            StatusCode::BAD_REQUEST,
            Json(ErrorResponse {
                error: format!("batch size {} exceeds maximum of {MAX_BATCH_SIZE}", req.texts.len()),
            }),
        ));
    }

    let encodings = req
        .texts
        .iter()
        .map(|t| state.tokenizer.encode(t.as_str(), true))
        .collect::<Result<Vec<_>, _>>()
        .map_err(|e| {
            error!("tokenization failed: {e}");
            err_500("tokenization failed")
        })?;

    let batch_size = encodings.len();
    let seq_len = encodings.iter().map(|e| e.get_ids().len()).max().unwrap_or(0);

    let mut input_ids = vec![0i64; batch_size * seq_len];
    let mut attention_mask = vec![0i64; batch_size * seq_len];
    let token_type_ids = vec![0i64; batch_size * seq_len];

    for (i, enc) in encodings.iter().enumerate() {
        let ids = enc.get_ids();
        let mask = enc.get_attention_mask();
        for (j, (&id, &m)) in ids.iter().zip(mask.iter()).enumerate() {
            input_ids[i * seq_len + j] = id as i64;
            attention_mask[i * seq_len + j] = m as i64;
        }
    }
    let _ = &token_type_ids; // all zeros for single-sentence

    let shape = vec![batch_size as i64, seq_len as i64];

    let ids_tensor = Tensor::from_array((shape.clone(), input_ids.clone()))
        .map_err(|e| { error!("tensor error: {e}"); err_500("tensor creation failed") })?;
    let mask_tensor = Tensor::from_array((shape.clone(), attention_mask.clone()))
        .map_err(|e| { error!("tensor error: {e}"); err_500("tensor creation failed") })?;
    let type_tensor = Tensor::from_array((shape, token_type_ids))
        .map_err(|e| { error!("tensor error: {e}"); err_500("tensor creation failed") })?;

    let inputs = ort::inputs![
        "input_ids" => ids_tensor,
        "attention_mask" => mask_tensor,
        "token_type_ids" => type_tensor,
    ];

    let mut session = state.session.lock().await;
    let outputs = session.run(inputs).map_err(|e| {
        error!("inference failed: {e}");
        err_500("inference failed")
    })?;

    let (output_shape, data) = outputs[0]
        .try_extract_tensor::<f32>()
        .map_err(|e| {
            error!("failed to extract tensor: {e}");
            err_500("output extraction failed")
        })?;

    let rank = output_shape.len();
    let embeddings = if rank == 3 {
        // [batch, seq_len, dim] — mean pooling with attention mask
        let dim = output_shape[2] as usize;
        let sl = output_shape[1] as usize;
        (0..batch_size)
            .map(|i| {
                let mut vec = vec![0f32; dim];
                let mut count = 0f32;
                for j in 0..sl {
                    if attention_mask[i * seq_len + j] == 1 {
                        let offset = i * sl * dim + j * dim;
                        for k in 0..dim {
                            vec[k] += data[offset + k];
                        }
                        count += 1.0;
                    }
                }
                if count > 0.0 {
                    vec.iter_mut().for_each(|x| *x /= count);
                }
                l2_normalize(&mut vec);
                vec
            })
            .collect()
    } else {
        // [batch, dim] — already pooled
        (0..batch_size)
            .map(|i| {
                let mut vec = data[i * DIMENSIONS..(i + 1) * DIMENSIONS].to_vec();
                l2_normalize(&mut vec);
                vec
            })
            .collect()
    };

    Ok(Json(EmbedResponse { embeddings }))
}

async fn health() -> Json<HealthResponse> {
    Json(HealthResponse {
        status: "ok",
        model: MODEL_NAME,
        dimensions: DIMENSIONS,
    })
}

async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c().await.expect("failed to install Ctrl+C handler");
    };
    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("failed to install SIGTERM handler")
            .recv()
            .await;
    };
    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
    info!("shutdown signal received");
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info".into()),
        )
        .init();

    let model_path = env::var("EMBED_MODEL_PATH")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("models/all-MiniLM-L6-v2/model.onnx"));

    let tokenizer_path = env::var("EMBED_TOKENIZER_PATH")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("models/all-MiniLM-L6-v2/tokenizer.json"));

    let port: u16 = env::var("EMBED_SVC_PORT")
        .ok()
        .and_then(|p| p.parse().ok())
        .unwrap_or(8900);

    info!("loading model from {}", model_path.display());
    let session = Session::builder()?
        .commit_from_file(&model_path)?;

    info!("loading tokenizer from {}", tokenizer_path.display());
    let tokenizer = Tokenizer::from_file(&tokenizer_path)
        .map_err(|e| anyhow::anyhow!("failed to load tokenizer: {e}"))?;

    let state = Arc::new(AppState {
        session: Mutex::new(session),
        tokenizer,
    });

    let app = Router::new()
        .route("/embed", post(embed))
        .route("/health", get(health))
        .layer(CorsLayer::permissive())
        .with_state(state);

    let addr = format!("0.0.0.0:{port}");
    info!("listening on {addr}");
    let listener = tokio::net::TcpListener::bind(&addr).await?;
    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;

    Ok(())
}
