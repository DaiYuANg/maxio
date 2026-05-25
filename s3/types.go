package s3

import "encoding/xml"

const defaultXMLNS = "http://s3.amazonaws.com/doc/2006-03-01/"

type owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type bucketResult struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type listAllMyBucketsResult struct {
	XMLName xml.Name       `xml:"ListAllMyBucketsResult"`
	XMLNS   string         `xml:"xmlns,attr,omitempty"`
	Owner   owner          `xml:"Owner"`
	Buckets []bucketResult `xml:"Buckets>Bucket"`
}

type objectResult struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type listBucketResult struct {
	XMLName     xml.Name       `xml:"ListBucketResult"`
	XMLNS       string         `xml:"xmlns,attr,omitempty"`
	Name        string         `xml:"Name"`
	Prefix      string         `xml:"Prefix"`
	KeyCount    int            `xml:"KeyCount"`
	MaxKeys     int            `xml:"MaxKeys"`
	IsTruncated bool           `xml:"IsTruncated"`
	Contents    []objectResult `xml:"Contents"`
}

type commonPrefixResult struct {
	Prefix string `xml:"Prefix"`
}

type listBucketV2Result struct {
	XMLName               xml.Name             `xml:"ListBucketResult"`
	XMLNS                 string               `xml:"xmlns,attr,omitempty"`
	Name                  string               `xml:"Name"`
	Prefix                string               `xml:"Prefix"`
	Delimiter             string               `xml:"Delimiter,omitempty"`
	KeyCount              int                  `xml:"KeyCount"`
	MaxKeys               int                  `xml:"MaxKeys"`
	IsTruncated           bool                 `xml:"IsTruncated"`
	ContinuationToken     string               `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string               `xml:"NextContinuationToken,omitempty"`
	StartAfter            string               `xml:"StartAfter,omitempty"`
	Contents              []objectResult       `xml:"Contents"`
	CommonPrefixes        []commonPrefixResult `xml:"CommonPrefixes"`
}

type errorResult struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestID string   `xml:"RequestId"`
}

type bucketLocationResult struct {
	XMLName  xml.Name `xml:"LocationConstraint"`
	XMLNS    string   `xml:"xmlns,attr,omitempty"`
	Location string   `xml:",chardata"`
}

type copyObjectResult struct {
	XMLName      xml.Name `xml:"CopyObjectResult"`
	LastModified string   `xml:"LastModified"`
	ETag         string   `xml:"ETag"`
}

type deleteObjectsRequest struct {
	Quiet   bool                 `xml:"Quiet"`
	Objects []deleteObjectTarget `xml:"Object"`
}

type deleteObjectTarget struct {
	Key string `xml:"Key"`
}

type deleteObjectsResult struct {
	XMLName xml.Name              `xml:"DeleteResult"`
	XMLNS   string                `xml:"xmlns,attr,omitempty"`
	Deleted []deletedObjectResult `xml:"Deleted,omitempty"`
	Errors  []deleteErrorResult   `xml:"Error,omitempty"`
}

type deletedObjectResult struct {
	Key string `xml:"Key"`
}

type deleteErrorResult struct {
	Key     string `xml:"Key"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}
