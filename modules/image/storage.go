package image

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type objectInfo struct {
	Body     io.ReadCloser
	Size     int64
	Range    string
	ETag     string
	Modified time.Time
}
type objectStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Open(context.Context, string, string, bool) (objectInfo, error)
	Delete(context.Context, string) error
}
type localStore struct{ root *os.Root }

func validKey(key string) bool {
	if key == "" || len(key) > 2048 || strings.ContainsAny(key, "\\\x00:") || strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") || strings.HasPrefix(key, "_multipart/") {
		return false
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
func (l *localStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if !validKey(key) {
		return ErrInvalid
	}
	parent := path.Dir(key)
	if parent != "." {
		if err := l.root.MkdirAll(parent, 0750); err != nil {
			return err
		}
	}
	temp := key + ".pending-" + uuid.NewString()
	f, err := l.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer l.root.Remove(temp)
	n, err := io.Copy(f, io.LimitReader(r, size+1))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if n != size {
		return errors.New("object size did not match upload")
	}
	// Link refuses to overwrite existing files, including unindexed source files.
	if err = l.root.Link(temp, key); err != nil {
		return err
	}
	return nil
}
func (l *localStore) Open(ctx context.Context, key, byteRange string, head bool) (objectInfo, error) {
	if !validKey(key) {
		return objectInfo{}, ErrInvalid
	}
	f, err := l.root.Open(key)
	if err != nil {
		return objectInfo{}, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		f.Close()
		if err == nil {
			err = ErrInvalid
		}
		return objectInfo{}, err
	}
	return objectInfo{Body: f, Size: st.Size(), Modified: st.ModTime()}, nil
}
func (l *localStore) Delete(ctx context.Context, key string) error {
	if !validKey(key) {
		return ErrInvalid
	}
	err := l.root.Remove(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

type Channel struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Endpoint  string `json:"endpoint,omitempty"`
	Bucket    string `json:"bucketName,omitempty"`
	Region    string `json:"region,omitempty"`
	AccessKey string `json:"accessKeyId,omitempty"`
	Secret    string `json:"secretAccessKey,omitempty"`
	PathStyle bool   `json:"pathStyle"`
	CDN       string `json:"cdnDomain,omitempty"`
	SavePath  string `json:"savePath,omitempty"`
	Quota     struct {
		Enabled   bool    `json:"enabled"`
		LimitGB   float64 `json:"limitGB"`
		Threshold float64 `json:"threshold"`
	} `json:"quota"`
}
type channelGroup struct {
	Channels    []Channel `json:"channels"`
	LoadBalance struct {
		Enabled bool `json:"enabled"`
	} `json:"loadBalance"`
}
type uploadConfig struct {
	Local channelGroup `json:"cfr2"`
	S3    channelGroup `json:"s3"`
}

type s3Store struct {
	client *s3.Client
	bucket string
}

func newS3Store(c Channel, transport *http.Client) (*s3Store, error) {
	if c.Endpoint == "" || c.Bucket == "" || c.AccessKey == "" || c.Secret == "" {
		return nil, errors.New("S3 channel credentials missing")
	}
	region := c.Region
	if region == "" {
		region = "auto"
	}
	if transport == nil {
		transport = defaultS3HTTPClient
	}
	client := s3.NewFromConfig(aws.Config{Region: region, Credentials: credentials.NewStaticCredentialsProvider(c.AccessKey, c.Secret, ""), HTTPClient: transport}, func(o *s3.Options) { o.BaseEndpoint = aws.String(c.Endpoint); o.UsePathStyle = c.PathStyle })
	return &s3Store{client, c.Bucket}, nil
}
// A whole-request client timeout would cut large transfers on slow links, so
// only connection setup and time-to-first-header are bounded; the request
// context bounds the transfer itself.
var defaultS3HTTPClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 2 * time.Minute,
	ExpectContinueTimeout: time.Second,
	IdleConnTimeout:       90 * time.Second,
	MaxIdleConnsPerHost:   16,
	ForceAttemptHTTP2:     true,
}}

type s3Identity struct {
	Endpoint, Bucket, Region, AccessKey, Secret string
	PathStyle                                   bool
}

// s3For reuses one client per distinct channel configuration, so edited
// credentials or endpoints naturally select a new client.
func (m *Module) s3For(c Channel) (*s3Store, error) {
	key := s3Identity{c.Endpoint, c.Bucket, c.Region, c.AccessKey, c.Secret, c.PathStyle}
	if cached, ok := m.s3Clients.Load(key); ok {
		return cached.(*s3Store), nil
	}
	st, err := newS3Store(c, m.deps.StorageHTTPClient)
	if err != nil {
		return nil, err
	}
	cached, _ := m.s3Clients.LoadOrStore(key, st)
	return cached.(*s3Store), nil
}

func (s *s3Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: r, ContentLength: aws.Int64(size), ContentType: aws.String(contentType), IfNoneMatch: aws.String("*")})
	return err
}
func (s *s3Store) Open(ctx context.Context, key, byteRange string, head bool) (objectInfo, error) {
	if head {
		v, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
		if err != nil {
			return objectInfo{}, err
		}
		return objectInfo{Size: aws.ToInt64(v.ContentLength), ETag: aws.ToString(v.ETag), Modified: aws.ToTime(v.LastModified)}, nil
	}
	input := &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if byteRange != "" {
		input.Range = aws.String(byteRange)
	}
	v, err := s.client.GetObject(ctx, input)
	if err != nil {
		return objectInfo{}, err
	}
	return objectInfo{Body: v.Body, Size: aws.ToInt64(v.ContentLength), Range: aws.ToString(v.ContentRange), ETag: aws.ToString(v.ETag), Modified: aws.ToTime(v.LastModified)}, nil
}
func (s *s3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return err
}

func (m *Module) channels(ctx context.Context) (uploadConfig, error) {
	return m.channelsWith(ctx, m.pool)
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (m *Module) channelsWith(ctx context.Context, q rowQuerier) (uploadConfig, error) {
	var raw string
	err := q.QueryRow(ctx, `SELECT value FROM image_settings WHERE key='manage@sysConfig@upload'`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return uploadConfig{}, err
	}
	var cfg uploadConfig
	if raw != "" {
		if err := decodeJSON([]byte(raw), &cfg); err != nil {
			return cfg, err
		}
	}
	// Preserve the original environment-backed local channel, with stored overrides.
	local := Channel{Name: "R2_env", Enabled: true, SavePath: "environment variable"}
	result := []Channel{local}
	for _, c := range cfg.Local.Channels {
		if c.SavePath == "environment variable" || c.Name == local.Name {
			result[0] = c
			result[0].Name = local.Name
		} else {
			result = append(result, c)
		}
	}
	cfg.Local.Channels = result
	return cfg, nil
}

func (m *Module) storeFor(ctx context.Context, meta map[string]any) (objectStore, string, error) {
	return m.storeForWith(ctx, meta, m.pool)
}
func (m *Module) storeForWith(ctx context.Context, meta map[string]any, q rowQuerier) (objectStore, string, error) {
	key := text(meta, "S3FileKey")
	switch text(meta, "Channel") {
	case "CloudflareR2":
		return m.local, "", nil
	case "S3":
		cfg, err := m.channelsWith(ctx, q)
		if err != nil {
			return nil, "", err
		}
		var matches []Channel
		for _, c := range cfg.S3.Channels {
			if text(meta, "ChannelName") != "" {
				if c.Name == text(meta, "ChannelName") {
					matches = []Channel{c}
					break
				}
				continue
			}
			if c.Endpoint == text(meta, "S3Endpoint") && c.Bucket == text(meta, "S3BucketName") {
				matches = append(matches, c)
			}
		}
		if len(matches) != 1 || key == "" {
			return nil, "", errors.New("S3 channel cannot be resolved unambiguously")
		}
		st, err := m.s3For(matches[0])
		if err != nil {
			return nil, "", err
		}
		return st, key, nil
	default:
		return nil, "", errors.New("unsupported historical storage channel")
	}
}
