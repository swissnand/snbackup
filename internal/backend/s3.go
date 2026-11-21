package backend

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// S3Config configures an S3-compatible endpoint (AWS S3, MinIO, Wasabi,
// Backblaze B2, Cloudflare R2, Exoscale SOS, Infomaniak Swiss Backup ...).
type S3Config struct {
	Endpoint     string `json:"endpoint,omitempty"` // e.g. https://s3.eu-central-1.amazonaws.com
	Region       string `json:"region,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	Prefix       string `json:"prefix,omitempty"`
	AccessKey    string `json:"accessKey,omitempty"`
	SecretKey    string `json:"secretKey,omitempty"`
	PathStyle    bool   `json:"pathStyle,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
	// Object Lock (WORM / immutability against ransomware).
	ObjectLockMode string `json:"objectLockMode,omitempty"` // GOVERNANCE | COMPLIANCE
	ObjectLockDays int    `json:"objectLockDays,omitempty"`
}

// S3 implements Backend with AWS Signature V4 using only the standard library.
type S3 struct {
	cfg    S3Config
	base   *url.URL
	client *http.Client
}

// NewS3 creates an S3 backend.
func NewS3(cfg S3Config) (*S3, error) {
	if cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("s3: bucket, access key and secret key are required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://s3." + cfg.Region + ".amazonaws.com"
	}
	if !strings.Contains(cfg.Endpoint, "://") {
		cfg.Endpoint = "https://" + cfg.Endpoint
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && !strings.HasPrefix(u.Host, "localhost") && !strings.HasPrefix(u.Host, "127.") {
		return nil, errors.New("s3: only https endpoints are allowed (except localhost)")
	}
	cfg.Prefix = strings.Trim(cfg.Prefix, "/")
	return &S3{cfg: cfg, base: u, client: &http.Client{Timeout: 10 * time.Minute}}, nil
}

func (s *S3) key(name string) string {
	if s.cfg.Prefix == "" {
		return name
	}
	return s.cfg.Prefix + "/" + name
}

func (s *S3) objectURL(key string, q url.Values) *url.URL {
	u := *s.base
	if s.cfg.PathStyle {
		u.Path = "/" + s.cfg.Bucket + "/" + key
	} else {
		u.Host = s.cfg.Bucket + "." + s.base.Host
		u.Path = "/" + key
	}
	if q != nil {
		u.RawQuery = encodeQuery(q)
	}
	return &u
}

func encodeQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		for _, v := range q[k] {
			parts = append(parts, awsEscape(k, true)+"="+awsEscape(v, true))
		}
	}
	return strings.Join(parts, "&")
}

func awsEscape(s string, encodeSlash bool) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else if c == '/' && !encodeSlash {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func (s *S3) sign(req *http.Request, payloadHash string) {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("Host", req.URL.Host)

	var names []string
	for k := range req.Header {
		lk := strings.ToLower(k)
		if lk == "host" || strings.HasPrefix(lk, "x-amz-") || lk == "content-md5" || lk == "content-type" {
			names = append(names, lk)
		}
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, n := range names {
		v := req.Header.Get(n)
		if n == "host" {
			v = req.URL.Host
		}
		ch.WriteString(n + ":" + strings.TrimSpace(v) + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{
		req.Method,
		awsEscape(req.URL.Path, false),
		req.URL.RawQuery,
		ch.String(),
		signed,
		payloadHash,
	}, "\n")
	scope := date + "/" + s.cfg.Region + "/s3/aws4_request"
	h := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(h[:])
	k := hmacSHA256([]byte("AWS4"+s.cfg.SecretKey), date)
	k = hmacSHA256(k, s.cfg.Region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.cfg.AccessKey+"/"+scope+
		", SignedHeaders="+signed+", Signature="+sig)
	req.Header.Del("Host")
}

func (s *S3) do(ctx context.Context, method string, u *url.URL, body []byte, hdr map[string]string) (*http.Response, error) {
	var rd io.Reader
	sum := sha256.Sum256(body)
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, err
	}
	// url.String() may re-encode; keep our canonical path/query.
	req.URL.Opaque = ""
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	s.sign(req, hex.EncodeToString(sum[:]))
	return s.client.Do(req)
}

func s3Error(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	var e struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	_ = xml.Unmarshal(b, &e)
	if e.Code != "" {
		return fmt.Errorf("s3: %s: %s (HTTP %d)", e.Code, e.Message, resp.StatusCode)
	}
	return fmt.Errorf("s3: HTTP %d", resp.StatusCode)
}

func (s *S3) Save(ctx context.Context, name string, data []byte) error {
	md := md5.Sum(data)
	hdr := map[string]string{
		"Content-Type": "application/octet-stream",
		"Content-MD5":  base64.StdEncoding.EncodeToString(md[:]),
	}
	if s.cfg.StorageClass != "" {
		hdr["x-amz-storage-class"] = s.cfg.StorageClass
	}
	// Keep the repository config/keys mutable; lock data and snapshots.
	if s.cfg.ObjectLockMode != "" && s.cfg.ObjectLockDays > 0 &&
		(strings.HasPrefix(name, "data/") || strings.HasPrefix(name, "snapshots/")) {
		hdr["x-amz-object-lock-mode"] = strings.ToUpper(s.cfg.ObjectLockMode)
		hdr["x-amz-object-lock-retain-until-date"] = time.Now().UTC().
			Add(time.Duration(s.cfg.ObjectLockDays) * 24 * time.Hour).Format(time.RFC3339)
	}
	resp, err := s.do(ctx, http.MethodPut, s.objectURL(s.key(name), nil), data, hdr)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return s3Error(resp)
	}
	drain(resp.Body)
	return nil
}

func (s *S3) Load(ctx context.Context, name string) ([]byte, error) {
	resp, err := s.do(ctx, http.MethodGet, s.objectURL(s.key(name), nil), nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		drain(resp.Body)
		return nil, ErrNotFound
	}
	if resp.StatusCode/100 != 2 {
		return nil, s3Error(resp)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (s *S3) Exists(ctx context.Context, name string) (bool, error) {
	resp, err := s.do(ctx, http.MethodHead, s.objectURL(s.key(name), nil), nil, nil)
	if err != nil {
		return false, err
	}
	drain(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode/100 != 2 {
		return false, fmt.Errorf("s3: HTTP %d", resp.StatusCode)
	}
	return true, nil
}

func (s *S3) Delete(ctx context.Context, name string) error {
	resp, err := s.do(ctx, http.MethodDelete, s.objectURL(s.key(name), nil), nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return s3Error(resp)
	}
	drain(resp.Body)
	return nil
}

func (s *S3) List(ctx context.Context, dir string) ([]FileInfo, error) {
	prefix := s.key(strings.Trim(dir, "/"))
	if prefix != "" {
		prefix += "/"
	}
	var out []FileInfo
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}, "max-keys": {"1000"}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		u := s.objectURL("", q)
		if s.cfg.PathStyle {
			u.Path = "/" + s.cfg.Bucket + "/"
		} else {
			u.Path = "/"
		}
		resp, err := s.do(ctx, http.MethodGet, u, nil, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 != 2 {
			return nil, s3Error(resp)
		}
		var res struct {
			Contents []struct {
				Key  string `xml:"Key"`
				Size int64  `xml:"Size"`
			} `xml:"Contents"`
			IsTruncated bool   `xml:"IsTruncated"`
			NextToken   string `xml:"NextContinuationToken"`
		}
		err = xml.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, c := range res.Contents {
			name := c.Key
			if s.cfg.Prefix != "" {
				name = strings.TrimPrefix(name, s.cfg.Prefix+"/")
			}
			out = append(out, FileInfo{Name: name, Size: c.Size})
		}
		if !res.IsTruncated || res.NextToken == "" {
			break
		}
		token = res.NextToken
	}
	return out, nil
}

func (s *S3) Location() string {
	return "s3:" + s.base.Host + "/" + s.cfg.Bucket + "/" + s.cfg.Prefix
}
func (s *S3) Close() error { return nil }
