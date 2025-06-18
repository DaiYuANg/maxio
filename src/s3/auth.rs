use hmac::digest::Digest;
use hmac::{Hmac, Mac};
use percent_encoding::{NON_ALPHANUMERIC, utf8_percent_encode};
use sha2::Sha256;
use std::collections::BTreeMap;
use time::OffsetDateTime;

type HmacSha256 = Hmac<Sha256>;

pub fn validate_signature(
  method: &str,
  uri: &str,
  query: &str,
  headers: &BTreeMap<String, String>,
  payload: &[u8],
  authorization: &str,
  x_amz_date: &str,
  secret_key: &str,
) -> bool {
  // 1. Extract credential & signed headers from Authorization
  let Some((_, params)) = authorization.split_once(" ") else {
    return false;
  };
  let mut credential = "";
  let mut signed_headers = "";
  let mut signature = "";

  for part in params.split(", ") {
    if let Some((key, value)) = part.split_once('=') {
      match key {
        "Credential" => credential = value,
        "SignedHeaders" => signed_headers = value,
        "Signature" => signature = value,
        _ => {}
      }
    }
  }

  // 2. Hash the payload
  let hashed_payload = hex::encode(Sha256::digest(payload));

  // 3. Build canonical headers
  let mut canonical_headers = String::new();
  for key in signed_headers.split(';') {
    if let Some(val) = headers.get(key) {
      canonical_headers.push_str(&format!("{}:{}\n", key.to_lowercase(), val.trim()));
    }
  }

  // 4. Build canonical request
  let canonical_request = format!(
    "{method}\n{uri}\n{query}\n{headers}\n{signed_headers}\n{hashed_payload}",
    method = method,
    uri = uri,
    query = query,
    headers = canonical_headers,
    signed_headers = signed_headers,
    hashed_payload = hashed_payload,
  );

  let hashed_request = hex::encode(Sha256::digest(canonical_request.as_bytes()));

  // 5. Parse credential scope
  let parts: Vec<&str> = credential.split('/').collect();
  if parts.len() < 5 {
    return false;
  }
  let (_access_key, date, region, service, _) = (parts[0], parts[1], parts[2], parts[3], parts[4]);

  let credential_scope = format!("{}/{}/{}/aws4_request", date, region, service);

  let string_to_sign = format!(
    "AWS4-HMAC-SHA256\n{x_amz_date}\n{scope}\n{hash}",
    x_amz_date = x_amz_date,
    scope = credential_scope,
    hash = hashed_request
  );

  // 6. Derive signing key
  let k_date = hmac_sign(format!("AWS4{}", secret_key).as_bytes(), date.as_bytes());
  let k_region = hmac_sign(&k_date, region.as_bytes());
  let k_service = hmac_sign(&k_region, service.as_bytes());
  let k_signing = hmac_sign(&k_service, b"aws4_request");

  // 7. Calculate signature
  let expected_signature = hex::encode(hmac_sign(&k_signing, string_to_sign.as_bytes()));

  // 8. Compare
  expected_signature == signature
}

fn hmac_sign(key: &[u8], msg: &[u8]) -> Vec<u8> {
  let mut mac = HmacSha256::new_from_slice(key).expect("HMAC can take key of any size");
  mac.update(msg);
  mac.finalize().into_bytes().to_vec()
}
