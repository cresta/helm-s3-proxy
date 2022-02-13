package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/cresta/zapctx"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type objectCache struct {
	key  string
	body []byte
	etag string
}

type cachedIndexFiles struct {
	indexFiles map[string]objectCache
	mu         sync.RWMutex
}

func (c *cachedIndexFiles) cacheIndex(key string, body []byte, etag string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.indexFiles == nil {
		c.indexFiles = make(map[string]objectCache)
	}
	c.indexFiles[key] = objectCache{key: key, body: body, etag: etag}
}

func (c *cachedIndexFiles) getEtag(key string) ([]byte, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.indexFiles == nil {
		return nil, ""
	}
	obj, exists := c.indexFiles[key]
	if !exists {
		return nil, ""
	}
	return obj.body, obj.etag
}

type BucketHandler struct {
	Bucket          string
	Downloader      *s3manager.Downloader
	Log             *zapctx.Logger
	ReplaceHTTPPath string
	cache           cachedIndexFiles
}

func (b *BucketHandler) Setup(mux *mux.Router) error {
	mux.PathPrefix("/").HandlerFunc(b.handlePath).Methods("GET").Name("GetObject")
	if err := b.verifyS3Downloader(); err != nil {
		return fmt.Errorf("failed to setup bucket handler: %w", err)
	}
	return nil
}

func (b *BucketHandler) fetchFile(ctx context.Context, path string) ([]byte, error) {
	logger := b.Log.With(zap.String("path", path))
	var buf bytes.Buffer
	cachedBody, existingEtag := b.cache.getEtag(path)
	req := &s3.GetObjectInput{
		Bucket: &b.Bucket,
		Key:    &path,
	}
	if existingEtag != "" {
		req.IfNoneMatch = &existingEtag
	}
	var ret *s3.GetObjectOutput
	ret, err := b.Downloader.S3.GetObjectWithContext(ctx, req)
	if err != nil {
		if errIsNotModified(err) {
			logger.Info(ctx, "cached result")
			return cachedBody, nil
		}
		return nil, fmt.Errorf("unable to fetch object %s: %w", path, err)
	}
	if _, err := io.Copy(&buf, ret.Body); err != nil {
		return nil, fmt.Errorf("unable to copy object %s: %w", path, err)
	}
	if strings.HasSuffix(path, ".yaml") {
		buf = b.replaceBucketPath(buf)
	}
	if ret.ETag != nil && strings.HasSuffix(path, ".yaml") {
		logger.Info(ctx, "caching result")
		b.cache.cacheIndex(path, buf.Bytes(), *ret.ETag)
	}

	return buf.Bytes(), nil
}

func (b *BucketHandler) handlePath(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	content, err := b.fetchFile(request.Context(), path)
	if err != nil {
		if errIsNoSuchKey(err) {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		b.Log.IfErr(err).Warn(request.Context(), "unable to fetch object")
		return
	}
	if _, err := io.Copy(writer, bytes.NewReader(content)); err != nil {
		b.Log.IfErr(err).Warn(request.Context(), "unable to write response")
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
}

func errIsNotModified(err error) bool {
	var tgt awserr.Error
	if !errors.As(err, &tgt) {
		return false
	}
	// https://github.com/aws/aws-sdk/issues/69
	return tgt.Code() == "NotModified"
}

func errIsNoSuchKey(err error) bool {
	var tgt awserr.Error
	if !errors.As(err, &tgt) {
		return false
	}
	return tgt.Code() == s3.ErrCodeNoSuchKey
}

func (b *BucketHandler) verifyS3Downloader() error {
	if _, err := b.fetchFile(context.Background(), "/verify_s3_downloader_works"); err != nil {
		if errIsNoSuchKey(err) {
			return nil
		}
		return fmt.Errorf("unable to fetch object and verify bucket: %w", err)
	}
	return nil
}

func (b *BucketHandler) replaceBucketPath(buf bytes.Buffer) bytes.Buffer {
	if b.ReplaceHTTPPath == "" {
		return buf
	}
	bufStr := buf.String()
	newStr := strings.ReplaceAll(bufStr, "s3://"+b.Bucket+"/", b.ReplaceHTTPPath+"/")
	return *bytes.NewBufferString(newStr)
}
