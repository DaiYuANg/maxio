package s3

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sigV4Algorithm = "AWS4-HMAC-SHA256"
	sigV4Request   = "aws4_request"
	s3ServiceName  = "s3"
	sigV4TimeFmt   = "20060102T150405Z"
)

const (
	sigV4ClockSkew          = 15 * time.Minute
	sigV4PresignMaxLifetime = 7 * 24 * time.Hour
)

var (
	errSigV4ConfigIncomplete = errors.New("s3 authentication is not fully configured")
	errSigV4MissingHeader    = errors.New("missing s3 authorization header")
	errSigV4InvalidHeader    = errors.New("invalid s3 authorization header")
	errSigV4AccessDenied     = errors.New("s3 signature verification failed")
	errSigV4UnsupportedScope = errors.New("unsupported s3 credential scope")
	errSigV4MissingDate      = errors.New("missing s3 request date")
)

type sigV4Authorization struct {
	accessKey     string
	date          string
	region        string
	service       string
	signedHeaders []string
	signature     string
}

func (s *Service) authorize(r *http.Request) error {
	accessKey, signingSecret, err := s.s3Credentials()
	if err != nil || accessKey == "" {
		return err
	}

	if isSigV4PresignedRequest(r) {
		return s.authorizePresigned(r, accessKey, signingSecret)
	}
	return s.authorizeHeader(r, accessKey, signingSecret)
}

func (s *Service) authorizeHeader(r *http.Request, accessKey, signingSecret string) error {
	auth, err := parseSigV4Authorization(r.Header.Get("Authorization"))
	if err != nil {
		return err
	}
	validateErr := s.validateSigV4Authorization(auth, accessKey)
	if validateErr != nil {
		return validateErr
	}

	amzDate, err := sigV4RequestDate(r, auth.date)
	if err != nil {
		return err
	}
	if !validSigV4Signature(r, auth, signingSecret, amzDate) {
		return errSigV4AccessDenied
	}

	return nil
}

func (s *Service) authorizePresigned(r *http.Request, accessKey, signingSecret string) error {
	auth, amzDate, err := parseSigV4Presigned(r)
	if err != nil {
		return err
	}
	validateErr := s.validateSigV4Authorization(auth, accessKey)
	if validateErr != nil {
		return validateErr
	}
	if !validSigV4PresignedSignature(r, auth, signingSecret, amzDate) {
		return errSigV4AccessDenied
	}
	return nil
}

func (s *Service) s3Credentials() (string, string, error) {
	accessKey := strings.TrimSpace(s.cfg.AccessKey)
	signingSecret := strings.TrimSpace(s.cfg.SecretKey)
	if accessKey == "" && signingSecret == "" {
		return "", "", nil
	}
	if accessKey == "" || signingSecret == "" {
		return "", "", errSigV4ConfigIncomplete
	}
	return accessKey, signingSecret, nil
}

func (s *Service) validateSigV4Authorization(auth sigV4Authorization, accessKey string) error {
	if auth.accessKey != accessKey {
		return errSigV4AccessDenied
	}

	region := strings.TrimSpace(s.cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	if auth.region != region || auth.service != s3ServiceName {
		return errSigV4UnsupportedScope
	}
	return nil
}

func sigV4RequestDate(r *http.Request, credentialDate string) (string, error) {
	amzDate := strings.TrimSpace(r.Header.Get("X-Amz-Date"))
	if amzDate == "" || !strings.HasPrefix(amzDate, credentialDate) {
		return "", errSigV4MissingDate
	}
	return amzDate, nil
}

func isSigV4PresignedRequest(r *http.Request) bool {
	values := r.URL.Query()
	return values.Get("X-Amz-Algorithm") == sigV4Algorithm || values.Get("X-Amz-Signature") != ""
}

func parseSigV4Authorization(header string) (sigV4Authorization, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return sigV4Authorization{}, errSigV4MissingHeader
	}
	if !strings.HasPrefix(header, sigV4Algorithm+" ") {
		return sigV4Authorization{}, errSigV4InvalidHeader
	}

	values := parseAuthorizationValues(strings.TrimSpace(strings.TrimPrefix(header, sigV4Algorithm)))
	credential := values["Credential"]
	signedHeaders := values["SignedHeaders"]
	signature := values["Signature"]
	return newSigV4Authorization(credential, signedHeaders, signature)
}

func parseSigV4Presigned(r *http.Request) (sigV4Authorization, string, error) {
	values := r.URL.Query()
	if values.Get("X-Amz-Algorithm") != sigV4Algorithm {
		return sigV4Authorization{}, "", errSigV4InvalidHeader
	}
	amzDate := strings.TrimSpace(values.Get("X-Amz-Date"))
	if amzDate == "" {
		return sigV4Authorization{}, "", errSigV4MissingDate
	}
	if err := validateSigV4PresignedLifetime(amzDate, values.Get("X-Amz-Expires")); err != nil {
		return sigV4Authorization{}, "", err
	}

	auth, err := newSigV4Authorization(
		values.Get("X-Amz-Credential"),
		values.Get("X-Amz-SignedHeaders"),
		values.Get("X-Amz-Signature"),
	)
	if err != nil {
		return sigV4Authorization{}, "", err
	}
	if !strings.HasPrefix(amzDate, auth.date) {
		return sigV4Authorization{}, "", errSigV4MissingDate
	}
	return auth, amzDate, nil
}

func newSigV4Authorization(credential, signedHeaders, signature string) (sigV4Authorization, error) {
	if credential == "" || signedHeaders == "" || signature == "" {
		return sigV4Authorization{}, errSigV4InvalidHeader
	}
	parts := strings.Split(credential, "/")
	if len(parts) != 5 || parts[4] != sigV4Request {
		return sigV4Authorization{}, errSigV4InvalidHeader
	}

	headers := parseSignedHeaders(signedHeaders)
	if len(headers) == 0 {
		return sigV4Authorization{}, errSigV4InvalidHeader
	}

	return sigV4Authorization{
		accessKey:     parts[0],
		date:          parts[1],
		region:        parts[2],
		service:       parts[3],
		signedHeaders: headers,
		signature:     signature,
	}, nil
}

func validateSigV4PresignedLifetime(amzDate, expires string) error {
	signedAt, err := time.Parse(sigV4TimeFmt, amzDate)
	if err != nil {
		return errSigV4InvalidHeader
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(expires), 10, 64)
	if err != nil || seconds < 0 {
		return errSigV4InvalidHeader
	}
	lifetime := time.Duration(seconds) * time.Second
	if lifetime > sigV4PresignMaxLifetime {
		return errSigV4InvalidHeader
	}
	now := time.Now().UTC()
	if now.Before(signedAt.Add(-sigV4ClockSkew)) || now.After(signedAt.Add(lifetime)) {
		return errSigV4AccessDenied
	}
	return nil
}

func parseAuthorizationValues(input string) map[string]string {
	values := make(map[string]string, 3)
	for segment := range strings.SplitSeq(input, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}

func parseSignedHeaders(input string) []string {
	seen := make(map[string]struct{})
	headers := make([]string, 0, strings.Count(input, ";")+1)
	for header := range strings.SplitSeq(input, ";") {
		header = strings.ToLower(strings.TrimSpace(header))
		if header == "" {
			continue
		}
		if _, ok := seen[header]; ok {
			continue
		}
		seen[header] = struct{}{}
		headers = append(headers, header)
	}
	sort.Strings(headers)
	return headers
}
