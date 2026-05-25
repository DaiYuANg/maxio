package s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

func validSigV4Signature(r *http.Request, auth sigV4Authorization, signingSecret, amzDate string) bool {
	return validSigV4SignatureWithQuery(r, auth, signingSecret, amzDate, r.URL.Query())
}

func validSigV4PresignedSignature(r *http.Request, auth sigV4Authorization, signingSecret, amzDate string) bool {
	return validSigV4SignatureWithQuery(r, auth, signingSecret, amzDate, sigV4QueryWithoutSignature(r.URL.Query()))
}

func validSigV4SignatureWithQuery(
	r *http.Request,
	auth sigV4Authorization,
	signingSecret string,
	amzDate string,
	query url.Values,
) bool {
	canonicalRequest := canonicalSigV4Request(r, auth.signedHeaders, query)
	credentialScope := strings.Join([]string{auth.date, auth.region, auth.service, sigV4Request}, "/")
	stringToSign := strings.Join([]string{
		sigV4Algorithm,
		amzDate,
		credentialScope,
		sha256Hex(canonicalRequest),
	}, "\n")
	expected := sigV4Signature(signingSecret, auth.date, auth.region, auth.service, stringToSign)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(strings.ToLower(auth.signature))) == 1
}

func canonicalSigV4Request(r *http.Request, signedHeaders []string, query url.Values) string {
	headers := make([]string, 0, len(signedHeaders))
	for _, header := range signedHeaders {
		headers = append(headers, header+":"+canonicalSigV4HeaderValue(r, header))
	}

	return strings.Join([]string{
		r.Method,
		canonicalSigV4URI(r),
		canonicalSigV4Query(query),
		strings.Join(headers, "\n") + "\n",
		strings.Join(signedHeaders, ";"),
		payloadHash(r),
	}, "\n")
}

func canonicalSigV4URI(r *http.Request) string {
	path := r.URL.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func canonicalSigV4Query(values url.Values) string {
	if len(values) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(values))
	for key, items := range values {
		if len(items) == 0 {
			pairs = append(pairs, sigV4Escape(key)+"=")
			continue
		}
		for _, value := range items {
			pairs = append(pairs, sigV4Escape(key)+"="+sigV4Escape(value))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

func sigV4QueryWithoutSignature(values url.Values) url.Values {
	query := make(url.Values, len(values))
	for key, items := range values {
		if key == "X-Amz-Signature" {
			continue
		}
		query[key] = append([]string(nil), items...)
	}
	return query
}

func canonicalSigV4HeaderValue(r *http.Request, header string) string {
	if header == "host" {
		return strings.ToLower(strings.TrimSpace(r.Host))
	}
	return strings.Join(strings.Fields(strings.Join(r.Header.Values(header), ",")), " ")
}

func payloadHash(r *http.Request) string {
	hash := strings.TrimSpace(r.Header.Get("X-Amz-Content-Sha256"))
	if hash == "" {
		return "UNSIGNED-PAYLOAD"
	}
	return hash
}

func sigV4Escape(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func sigV4Signature(signingSecret, date, region, service, stringToSign string) string {
	dateKey := hmacSHA256([]byte("AWS4"+signingSecret), date)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, service)
	signingKey := hmacSHA256(serviceKey, sigV4Request)
	return hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write([]byte(data)); err != nil {
		return nil
	}
	return mac.Sum(nil)
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
