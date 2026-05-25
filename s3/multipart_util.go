package s3

import (
	"context"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/afero"
)

const (
	defaultMaxMultipartParts = 1000
	maxMultipartParts        = 1000
	minMultipartPartSize     = 5 * 1024 * 1024
)

type listPartsOptions struct {
	PartNumberMarker int
	MaxParts         int
}

type multipartPartHasher struct {
	sum *protocolMD5
}

func newMultipartPartHasher() *multipartPartHasher {
	return &multipartPartHasher{sum: newProtocolMD5()}
}

func (h *multipartPartHasher) Write(data []byte) (int, error) {
	return h.sum.Write(data)
}

func (h *multipartPartHasher) ETag() string {
	sum := h.sum.Sum()
	return hex.EncodeToString(sum[:])
}

func (h *multipartPartHasher) Digest() string {
	return h.ETag()
}

func multipartCompleteETag(parts []multipartPart) string {
	digests := make([]byte, 0, len(parts)*protocolMD5Size)
	for _, part := range parts {
		digest, err := hex.DecodeString(strings.Trim(part.Digest, `"`))
		if err != nil || len(digest) != protocolMD5Size {
			return fallbackMultipartETag(parts)
		}
		digests = append(digests, digest...)
	}
	sum := protocolMD5Sum(digests)
	return quoteETag(hex.EncodeToString(sum[:]) + "-" + strconv.Itoa(len(parts)))
}

func fallbackMultipartETag(parts []multipartPart) string {
	hasher := newProtocolMD5()
	for _, part := range parts {
		if _, err := hasher.Write([]byte(part.ETag)); err != nil {
			return quoteETag(strconv.Itoa(len(parts)))
		}
	}
	sum := hasher.Sum()
	return quoteETag(hex.EncodeToString(sum[:]) + "-" + strconv.Itoa(len(parts)))
}

func completeParts(upload multipartUpload, requested []completeMultipartPart) ([]multipartPart, error) {
	if len(requested) == 0 {
		return nil, errInvalidPart
	}
	parts := make([]multipartPart, 0, len(requested))
	previous := 0
	for _, item := range requested {
		if item.PartNumber <= previous {
			return nil, errInvalidPartOrder
		}
		part, ok := upload.Parts[item.PartNumber]
		if !ok || !etagMatches(part.ETag, item.ETag) {
			return nil, errInvalidPart
		}
		parts = append(parts, ensurePartDigest(part))
		previous = item.PartNumber
	}
	if err := validateMultipartPartSizes(parts); err != nil {
		return nil, err
	}
	return parts, nil
}

func validateMultipartPartSizes(parts []multipartPart) error {
	if len(parts) < 2 {
		return nil
	}
	for index := range len(parts) - 1 {
		if parts[index].Size < minMultipartPartSize {
			return errEntityTooSmall
		}
	}
	return nil
}

func sortedMultipartParts(parts map[int]multipartPart) []multipartPart {
	numbers := make([]int, 0, len(parts))
	for number := range parts {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	result := make([]multipartPart, 0, len(numbers))
	for _, number := range numbers {
		result = append(result, parts[number])
	}
	return result
}

func decodeCompleteMultipartUpload(reader io.Reader) (completeMultipartUploadRequest, error) {
	request := completeMultipartUploadRequest{}
	if reader == nil {
		return request, errMalformedXML
	}
	if err := xml.NewDecoder(reader).Decode(&request); err != nil {
		return request, fmt.Errorf("%w: decode complete multipart upload: %w", errMalformedXML, err)
	}
	return request, nil
}

func listPartsOptionsFromQuery(query url.Values) (listPartsOptions, error) {
	options := listPartsOptions{MaxParts: defaultMaxMultipartParts}
	if marker := queryValue(query, "part-number-marker"); marker != "" {
		value, err := strconv.Atoi(marker)
		if err != nil || value < 0 {
			return listPartsOptions{}, errInvalidPart
		}
		options.PartNumberMarker = value
	}
	if maxParts := queryValue(query, "max-parts"); maxParts != "" {
		value, err := strconv.Atoi(maxParts)
		if err != nil || value < 1 {
			return listPartsOptions{}, errInvalidPart
		}
		if value > maxMultipartParts {
			value = maxMultipartParts
		}
		options.MaxParts = value
	}
	return options, nil
}

func paginateMultipartParts(parts []multipartPart, options listPartsOptions) ([]multipartPart, int, bool) {
	filtered := make([]multipartPart, 0, len(parts))
	for _, part := range parts {
		if part.Number <= options.PartNumberMarker {
			continue
		}
		filtered = append(filtered, part)
	}
	if len(filtered) <= options.MaxParts {
		return filtered, 0, false
	}
	page := filtered[:options.MaxParts]
	return page, page[len(page)-1].Number, true
}

func cloneMultipartUserMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		output[key] = value
	}
	if len(output) == 0 {
		return nil
	}
	return output
}

func parsePartNumber(query url.Values) (int, error) {
	value := queryValue(query, "partNumber")
	if value == "" {
		return 0, errInvalidPart
	}
	partNumber, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%w: parse part number: %w", errInvalidPart, err)
	}
	if partNumber < 1 || partNumber > 10000 {
		return 0, errInvalidPart
	}
	return partNumber, nil
}

func validateUploadID(uploadID string) error {
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" || len(uploadID) > 128 || strings.IndexFunc(uploadID, invalidUploadIDRune) >= 0 {
		return errInvalidUploadID
	}
	return nil
}

func invalidUploadIDRune(value rune) bool {
	if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9') {
		return false
	}
	return value != '-' && value != '_'
}

func hasQueryKey(query url.Values, name string) bool {
	for key := range query {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func queryValue(query url.Values, name string) string {
	for key, values := range query {
		if !strings.EqualFold(key, name) || len(values) == 0 {
			continue
		}
		return values[0]
	}
	return ""
}

func quoteETag(value string) string {
	value = strings.Trim(value, `"`)
	return `"` + value + `"`
}

func etagMatches(stored, requested string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return true
	}
	return strings.Trim(stored, `"`) == strings.Trim(requested, `"`)
}

func ensurePartDigest(part multipartPart) multipartPart {
	if strings.TrimSpace(part.Digest) == "" {
		part.Digest = strings.Trim(part.ETag, `"`)
	}
	return part
}

func (a assembledMultipart) close() error {
	if a.file == nil {
		return nil
	}
	if err := a.file.Close(); err != nil {
		return fmt.Errorf("close multipart assembly: %w", err)
	}
	return nil
}

func closeFile(file interface{ Close() error }) {
	if file == nil {
		return
	}
	if err := file.Close(); err != nil {
		_ = err.Error()
	}
}

func closeAferoFile(file afero.File) {
	closeFile(file)
}

func contextError(ctx context.Context, operation string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s context: %w", operation, err)
	}
	return nil
}
