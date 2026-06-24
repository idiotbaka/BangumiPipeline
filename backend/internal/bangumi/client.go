package bangumi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bangumipipeline.local/server/internal/system"
)

const (
	jsonResponseLimit  = 25 << 20
	imageResponseLimit = 20 << 20
)

type apiClient struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
	logger     *slog.Logger
	limiter    *apiLimiter
}

type apiLimiter struct {
	mu          sync.Mutex
	nextRequest time.Time
	interval    time.Duration
}

func newAPILimiter(interval time.Duration) *apiLimiter {
	return &apiLimiter{interval: interval}
}

func newAPIClient(settings system.NetworkSettings, config SyncerConfig, logger *slog.Logger, limiter *apiLimiter) (*apiClient, error) {
	httpProxy, err := parseOptionalURL(settings.HTTPProxy)
	if err != nil {
		return nil, fmt.Errorf("HTTP 代理配置无效: %w", err)
	}
	httpsProxy, err := parseOptionalURL(settings.HTTPSProxy)
	if err != nil {
		return nil, fmt.Errorf("HTTPS 代理配置无效: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = config.RequestTimeout
	transport.Proxy = func(request *http.Request) (*url.URL, error) {
		if request.URL.Scheme == "https" {
			return httpsProxy, nil
		}
		return httpProxy, nil
	}
	return &apiClient{
		httpClient: &http.Client{Transport: transport, Timeout: config.RequestTimeout},
		baseURL:    strings.TrimRight(config.APIBaseURL, "/"), userAgent: config.UserAgent,
		limiter: limiter, logger: logger,
	}, nil
}

func (c *apiClient) close() {
	c.httpClient.CloseIdleConnections()
}

func (c *apiClient) getJSON(ctx context.Context, path string, target any) error {
	c.logger.Info("API 抓取中", "source", "bangumi", "endpoint", path)
	if err := c.waitForAPISlot(ctx); err != nil {
		c.logger.Error("API 抓取失败", "source", "bangumi", "endpoint", path, "error", err)
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		c.logger.Error("API 抓取失败", "source", "bangumi", "endpoint", path, "error", err)
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("API 抓取失败", "source", "bangumi", "endpoint", path, "error", err)
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		err := fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		c.logger.Error("API 抓取失败", "source", "bangumi", "endpoint", path, "error", err)
		return err
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, jsonResponseLimit)).Decode(target); err != nil {
		c.logger.Error("API 抓取失败", "source", "bangumi", "endpoint", path, "error", err)
		return err
	}
	c.logger.Info("API 抓取成功", "source", "bangumi", "endpoint", path)
	return nil
}

func (c *apiClient) waitForAPISlot(ctx context.Context) error {
	if c.limiter == nil {
		return nil
	}
	return c.limiter.wait(ctx)
}

func (l *apiLimiter) wait(ctx context.Context) error {
	l.mu.Lock()
	slot := time.Now()
	if l.nextRequest.After(slot) {
		slot = l.nextRequest
	}
	l.nextRequest = slot.Add(l.interval)
	l.mu.Unlock()
	wait := time.Until(slot)
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func (c *apiClient) downloadImage(ctx context.Context, sourceURL, destinationDir, baseName string) (imageDownload, error) {
	if sourceURL == "" {
		c.logger.Warn("图片资源不存在", "source", "bangumi", "file", baseName, "reason", "empty URL")
		return imageDownload{Status: imageStatusNotFound}, nil
	}
	c.logger.Info("图片下载中", "source", "bangumi", "url", sourceURL, "file", baseName)
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		err = fmt.Errorf("创建图片目录: %w", err)
		c.logImageFailure(sourceURL, baseName, err)
		return imageDownload{Status: imageStatusFailed}, err
	}
	extension := imageExtension(sourceURL, "")
	destination := filepath.Join(destinationDir, baseName+extension)
	if info, err := os.Stat(destination); err == nil && info.Size() > 0 {
		c.logger.Info("图片下载成功", "source", "bangumi", "url", sourceURL, "path", destination, "cached", true)
		return imageDownload{Path: destination, Status: imageStatusDownloaded}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		c.logImageFailure(sourceURL, baseName, err)
		return imageDownload{Status: imageStatusFailed}, err
	}
	req.Header.Set("Accept", "image/*")
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.httpClient.Do(req)
	if err != nil {
		c.logImageFailure(sourceURL, baseName, err)
		return imageDownload{Status: imageStatusFailed}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		c.logger.Warn("图片下载失败", "source", "bangumi", "url", sourceURL, "file", baseName, "error", "HTTP 404; no retry")
		return imageDownload{Status: imageStatusNotFound}, nil
	}
	if response.StatusCode != http.StatusOK {
		err := fmt.Errorf("HTTP %d", response.StatusCode)
		c.logImageFailure(sourceURL, baseName, err)
		return imageDownload{Status: imageStatusFailed}, err
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType != "" && !strings.HasPrefix(mediaType, "image/") {
		err := fmt.Errorf("响应不是图片: %s", mediaType)
		c.logImageFailure(sourceURL, baseName, err)
		return imageDownload{Status: imageStatusFailed}, err
	}
	if extension == ".jpg" {
		extension = imageExtension(sourceURL, mediaType)
		destination = filepath.Join(destinationDir, baseName+extension)
	}

	temporary, err := os.CreateTemp(destinationDir, ".image-*.tmp")
	if err != nil {
		c.logImageFailure(sourceURL, baseName, err)
		return imageDownload{Status: imageStatusFailed}, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	written, copyErr := io.Copy(temporary, io.LimitReader(response.Body, imageResponseLimit+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		c.logImageFailure(sourceURL, baseName, copyErr)
		return imageDownload{Status: imageStatusFailed}, copyErr
	}
	if closeErr != nil {
		c.logImageFailure(sourceURL, baseName, closeErr)
		return imageDownload{Status: imageStatusFailed}, closeErr
	}
	if written > imageResponseLimit {
		err := fmt.Errorf("图片超过 %d MiB 限制", imageResponseLimit>>20)
		c.logImageFailure(sourceURL, baseName, err)
		return imageDownload{Status: imageStatusFailed}, err
	}
	if written == 0 {
		err := errors.New("图片内容为空")
		c.logImageFailure(sourceURL, baseName, err)
		return imageDownload{Status: imageStatusFailed}, err
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		c.logImageFailure(sourceURL, baseName, err)
		return imageDownload{Status: imageStatusFailed}, err
	}
	c.logger.Info("图片下载成功", "source", "bangumi", "url", sourceURL, "path", destination, "bytes", written)
	return imageDownload{Path: destination, Status: imageStatusDownloaded}, nil
}

func (c *apiClient) logImageFailure(sourceURL, baseName string, err error) {
	c.logger.Error("图片下载失败", "source", "bangumi", "url", sourceURL, "file", baseName, "error", err)
}

func parseOptionalURL(value string) (*url.URL, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("proxy must be an HTTP or HTTPS URL")
	}
	return parsed, nil
}

func imageExtension(sourceURL, contentType string) string {
	if parsed, err := url.Parse(sourceURL); err == nil {
		extension := strings.ToLower(filepath.Ext(parsed.Path))
		switch extension {
		case ".jpg", ".jpeg", ".png", ".webp":
			return extension
		}
	}
	switch strings.ToLower(strings.Split(contentType, ";")[0]) {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}
