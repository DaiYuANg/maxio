package s3

import (
	"encoding/xml"
	"os"
	"sync"
	"time"
)

type multipartStore struct {
	root string
	mu   sync.Mutex
}

type multipartUpload struct {
	UploadID           string                `json:"upload_id"`
	Bucket             string                `json:"bucket"`
	Key                string                `json:"key"`
	ContentType        string                `json:"content_type"`
	CacheControl       string                `json:"cache_control,omitempty"`
	ContentDisposition string                `json:"content_disposition,omitempty"`
	ContentEncoding    string                `json:"content_encoding,omitempty"`
	ContentLanguage    string                `json:"content_language,omitempty"`
	UserMetadata       map[string]string     `json:"user_metadata,omitempty"`
	CreatedAt          time.Time             `json:"created_at"`
	Parts              map[int]multipartPart `json:"parts"`
}

type multipartPart struct {
	Number     int       `json:"number"`
	ETag       string    `json:"etag"`
	Digest     string    `json:"digest"`
	Size       int64     `json:"size"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type assembledMultipart struct {
	file *os.File
	etag string
}

type initiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	XMLNS    string   `xml:"xmlns,attr,omitempty"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

type completeMultipartUploadRequest struct {
	Parts []completeMultipartPart `xml:"Part"`
}

type completeMultipartPart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type completeMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	XMLNS    string   `xml:"xmlns,attr,omitempty"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

type listPartsResult struct {
	XMLName              xml.Name         `xml:"ListPartsResult"`
	XMLNS                string           `xml:"xmlns,attr,omitempty"`
	Bucket               string           `xml:"Bucket"`
	Key                  string           `xml:"Key"`
	UploadID             string           `xml:"UploadId"`
	PartNumberMarker     int              `xml:"PartNumberMarker"`
	NextPartNumberMarker int              `xml:"NextPartNumberMarker,omitempty"`
	MaxParts             int              `xml:"MaxParts"`
	IsTruncated          bool             `xml:"IsTruncated"`
	Parts                []partItemResult `xml:"Part"`
}

type partItemResult struct {
	PartNumber   int    `xml:"PartNumber"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

type listMultipartUploadsResult struct {
	XMLName            xml.Name                    `xml:"ListMultipartUploadsResult"`
	XMLNS              string                      `xml:"xmlns,attr,omitempty"`
	Bucket             string                      `xml:"Bucket"`
	Prefix             string                      `xml:"Prefix,omitempty"`
	KeyMarker          string                      `xml:"KeyMarker,omitempty"`
	UploadIDMarker     string                      `xml:"UploadIdMarker,omitempty"`
	NextKeyMarker      string                      `xml:"NextKeyMarker,omitempty"`
	NextUploadIDMarker string                      `xml:"NextUploadIdMarker,omitempty"`
	MaxUploads         int                         `xml:"MaxUploads"`
	IsTruncated        bool                        `xml:"IsTruncated"`
	Uploads            []multipartUploadItemResult `xml:"Upload"`
}

type multipartUploadItemResult struct {
	Key       string `xml:"Key"`
	UploadID  string `xml:"UploadId"`
	Initiated string `xml:"Initiated"`
}
