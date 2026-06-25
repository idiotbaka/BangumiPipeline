package download

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"bangumipipeline.local/server/internal/system"
)

const (
	commonQBitTag = "bangumi-pipeline"
	qBitTimeout   = 20 * time.Second
)

type qBitClient struct {
	baseURL string
	client  *http.Client
}

type qBitTorrent struct {
	Hash          string  `json:"hash"`
	Name          string  `json:"name"`
	State         string  `json:"state"`
	SavePath      string  `json:"save_path"`
	Progress      float64 `json:"progress"`
	Size          int64   `json:"size"`
	Downloaded    int64   `json:"downloaded"`
	DownloadSpeed int64   `json:"dlspeed"`
	Tags          string  `json:"tags"`
}

func newQBitClient(settings system.DownloadSettings) (*qBitClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &qBitClient{
		baseURL: fmt.Sprintf("http://%s:%d", settings.Host, settings.Port),
		client:  &http.Client{Jar: jar, Timeout: qBitTimeout},
	}, nil
}

func (c *qBitClient) login(ctx context.Context, username, password string) error {
	values := url.Values{}
	values.Set("username", username)
	values.Set("password", password)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/auth/login", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	message := strings.TrimSpace(string(body))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("qBittorrent login failed: HTTP %d %s", response.StatusCode, message)
	}
	if message != "" && message != "Ok." {
		return fmt.Errorf("qBittorrent login failed: HTTP %d %s", response.StatusCode, message)
	}
	return nil
}

func (c *qBitClient) version(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/app/version", nil)
	if err != nil {
		return "", err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("qBittorrent version failed: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return strings.TrimSpace(string(body)), nil
}

func (c *qBitClient) addURL(ctx context.Context, sourceURL, savePath string, tags []string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"urls":     sourceURL,
		"savepath": savePath,
		"tags":     strings.Join(tags, ","),
		"paused":   "false",
		"autoTMM":  "false",
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/torrents/add", &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("qBittorrent add failed: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if strings.Contains(strings.ToLower(string(responseBody)), "fails") {
		return fmt.Errorf("qBittorrent add failed: %s", strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func (c *qBitClient) deleteTorrents(ctx context.Context, hashes []string, deleteFiles bool) error {
	normalized := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		if hash != "" {
			normalized = append(normalized, hash)
		}
	}
	if len(normalized) == 0 {
		return errors.New("qBittorrent torrent hash is empty")
	}
	values := url.Values{}
	values.Set("hashes", strings.Join(normalized, "|"))
	values.Set("deleteFiles", fmt.Sprintf("%t", deleteFiles))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/torrents/delete", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("qBittorrent delete failed: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *qBitClient) torrentsByTag(ctx context.Context, tag string) ([]qBitTorrent, error) {
	endpoint := c.baseURL + "/api/v2/torrents/info?tag=" + url.QueryEscape(tag)
	return c.fetchTorrents(ctx, endpoint)
}

func (c *qBitClient) torrents(ctx context.Context) ([]qBitTorrent, error) {
	return c.fetchTorrents(ctx, c.baseURL+"/api/v2/torrents/info")
}

func (c *qBitClient) fetchTorrents(ctx context.Context, endpoint string) ([]qBitTorrent, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qBittorrent torrents info failed: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var torrents []qBitTorrent
	if err := json.Unmarshal(body, &torrents); err != nil {
		return nil, err
	}
	return torrents, nil
}

func tagForItem(itemID int64) string {
	return fmt.Sprintf("bp-item-%d", itemID)
}

func torrentHasTag(torrent qBitTorrent, tag string) bool {
	for _, value := range strings.Split(torrent.Tags, ",") {
		if strings.TrimSpace(value) == tag {
			return true
		}
	}
	return false
}
