package bangumi

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"bangumipipeline.local/server/internal/system"
)

const (
	BangumiSmileManifestVersion = 1
	BangumiSmileAssetCount      = 428

	defaultBangumiWebBaseURL  = "https://bangumi.tv"
	defaultBangumiLainBaseURL = "https://lain.bgm.tv"
	defaultSmileTimeout       = 20 * time.Second
	maxSmileResponseBytes     = 5 << 20
	smileManifestFilename     = "manifest.json"
)

var (
	smileCodePattern = regexp.MustCompile(`\((?:bgm[0-9]{2,3}|musume_[0-9]{2,3}|blake_[0-9]{2,3})\)`)
	knownSmileCodes  = func() map[string]struct{} {
		definitions := bangumiSmileDefinitions(defaultBangumiWebBaseURL, defaultBangumiLainBaseURL)
		result := make(map[string]struct{}, len(definitions))
		for _, definition := range definitions {
			result[definition.Code] = struct{}{}
		}
		return result
	}()
)

type BangumiSmileSyncConfig struct {
	Directory      string
	BangumiBaseURL string
	LainBaseURL    string
	UserAgent      string
	RequestTimeout time.Duration
}

type BangumiSmileAsset struct {
	Code        string `json:"code"`
	File        string `json:"file"`
	ContentType string `json:"contentType"`
	SourceURL   string `json:"sourceUrl"`
}

type BangumiSmileManifest struct {
	Version       int                          `json:"version"`
	GeneratedAt   int64                        `json:"generatedAt"`
	Complete      bool                         `json:"complete"`
	ExpectedCount int                          `json:"expectedCount"`
	Assets        map[string]BangumiSmileAsset `json:"assets"`
}

type BangumiSmileMatch struct {
	Code  string
	Start int
	End   int
}

type BangumiSmileSyncResult struct {
	Directory  string
	Expected   int
	Available  int
	Downloaded int
	Cached     int
	Complete   bool
}

type bangumiSmileDefinition struct {
	Code       string
	SourceURLs []string
}

// BangumiSmileStore downloads the original GIF/PNG files without converting
// them, preserving animated smiles. A manifest binds comment codes to local
// files so callers never need to infer an ambiguous extension.
type BangumiSmileStore struct {
	logger *slog.Logger
	config BangumiSmileSyncConfig
	mu     sync.Mutex
}

func NewBangumiSmileStore(logger *slog.Logger, config BangumiSmileSyncConfig) *BangumiSmileStore {
	config.Directory = filepath.Clean(strings.TrimSpace(config.Directory))
	config.BangumiBaseURL = strings.TrimRight(strings.TrimSpace(config.BangumiBaseURL), "/")
	if config.BangumiBaseURL == "" {
		config.BangumiBaseURL = defaultBangumiWebBaseURL
	}
	config.LainBaseURL = strings.TrimRight(strings.TrimSpace(config.LainBaseURL), "/")
	if config.LainBaseURL == "" {
		config.LainBaseURL = defaultBangumiLainBaseURL
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = defaultSmileTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &BangumiSmileStore{logger: logger, config: config}
}

// MatchBangumiSmileCodes locates supported smile codes in a comment. Offsets
// are byte offsets, matching Go string slicing and the JSON/UTF-8 payload.
func MatchBangumiSmileCodes(content string) []BangumiSmileMatch {
	indices := smileCodePattern.FindAllStringIndex(content, -1)
	matches := make([]BangumiSmileMatch, 0, len(indices))
	for _, index := range indices {
		code := content[index[0]:index[1]]
		if !IsBangumiSmileCode(code) {
			continue
		}
		matches = append(matches, BangumiSmileMatch{Code: code, Start: index[0], End: index[1]})
	}
	return matches
}

func IsBangumiSmileCode(code string) bool {
	_, ok := knownSmileCodes[code]
	return ok
}

func LoadBangumiSmileManifest(directory string) (BangumiSmileManifest, error) {
	data, err := os.ReadFile(filepath.Join(directory, smileManifestFilename))
	if err != nil {
		return BangumiSmileManifest{}, err
	}
	var manifest BangumiSmileManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return BangumiSmileManifest{}, fmt.Errorf("解析 Bangumi 表情清单: %w", err)
	}
	if manifest.Assets == nil {
		manifest.Assets = make(map[string]BangumiSmileAsset)
	}
	return manifest, nil
}

// HasCompleteManifest performs the cheap startup check used by the scheduled
// comment task. A complete manifest is the durable marker written only after
// every expected asset has been downloaded and validated; the task therefore
// does not reopen hundreds of immutable image files on every run or restart.
func (s *BangumiSmileStore) HasCompleteManifest() bool {
	manifest, err := LoadBangumiSmileManifest(s.config.Directory)
	if err != nil || manifest.Version != BangumiSmileManifestVersion || !manifest.Complete {
		return false
	}
	definitions := bangumiSmileDefinitions(s.config.BangumiBaseURL, s.config.LainBaseURL)
	if manifest.ExpectedCount != len(definitions) || len(manifest.Assets) != len(definitions) {
		return false
	}
	for _, definition := range definitions {
		asset, _, ok := manifest.Resolve(s.config.Directory, definition.Code)
		if !ok {
			return false
		}
		switch filepath.Ext(asset.File) {
		case ".gif":
			if asset.ContentType != "image/gif" {
				return false
			}
		case ".png":
			if asset.ContentType != "image/png" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Resolve returns the manifest entry and its local path. It only accepts an
// expected code and a safe basename produced by the smile synchronizer.
func (m BangumiSmileManifest) Resolve(directory, code string) (BangumiSmileAsset, string, bool) {
	asset, ok := m.Assets[code]
	wantBase := smileAssetBaseName(code)
	validFilename := asset.File == wantBase+".gif" || asset.File == wantBase+".png"
	if m.Version != BangumiSmileManifestVersion || !IsBangumiSmileCode(code) || !ok || asset.Code != code || !validFilename {
		return BangumiSmileAsset{}, "", false
	}
	return asset, filepath.Join(directory, asset.File), true
}

func (s *BangumiSmileStore) Ensure(ctx context.Context, network system.NetworkSettings) (BangumiSmileSyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definitions := bangumiSmileDefinitions(s.config.BangumiBaseURL, s.config.LainBaseURL)
	result := BangumiSmileSyncResult{Directory: s.config.Directory, Expected: len(definitions)}
	if strings.TrimSpace(s.config.Directory) == "" || s.config.Directory == "." {
		return result, errors.New("Bangumi 表情保存目录不能为空")
	}
	if err := os.MkdirAll(s.config.Directory, 0o755); err != nil {
		return result, fmt.Errorf("创建 Bangumi 表情目录: %w", err)
	}

	manifest, loadErr := LoadBangumiSmileManifest(s.config.Directory)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		s.logger.Warn("Bangumi 表情清单无效，将重新校验本地文件", "source", "bangumi", "error", loadErr)
	}
	if manifest.Version != BangumiSmileManifestVersion || manifest.Assets == nil {
		manifest = BangumiSmileManifest{Version: BangumiSmileManifestVersion, Assets: make(map[string]BangumiSmileAsset)}
	}

	valid := make(map[string]BangumiSmileAsset, len(definitions))
	missing := make([]bangumiSmileDefinition, 0)
	for _, definition := range definitions {
		if asset, ok := s.validManifestAsset(manifest, definition.Code); ok {
			valid[definition.Code] = asset
			result.Cached++
			continue
		}
		if asset, ok := s.findExistingAsset(definition); ok {
			valid[definition.Code] = asset
			result.Cached++
			continue
		}
		missing = append(missing, definition)
	}
	if len(missing) == 0 {
		result.Available = len(valid)
		result.Complete = true
		if manifestIsComplete(manifest, valid, len(definitions)) {
			return result, nil
		}
		if err := s.writeManifest(valid, true, len(definitions)); err != nil {
			return result, err
		}
		return result, nil
	}

	httpClient, err := newBangumiSmileHTTPClient(network, s.config.RequestTimeout)
	if err != nil {
		return result, err
	}
	defer httpClient.CloseIdleConnections()

	failures := make([]string, 0)
	for _, definition := range missing {
		asset, downloadErr := s.download(ctx, httpClient, definition)
		if downloadErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", definition.Code, downloadErr))
			continue
		}
		valid[definition.Code] = asset
		result.Downloaded++
	}
	result.Available = len(valid)
	result.Complete = len(valid) == len(definitions)
	if err := s.writeManifest(valid, result.Complete, len(definitions)); err != nil {
		failures = append(failures, err.Error())
	}
	s.logger.Info("Bangumi 评论表情资源同步完成", "source", "bangumi",
		"expected", result.Expected, "available", result.Available,
		"downloaded", result.Downloaded, "cached", result.Cached, "directory", result.Directory)
	if len(failures) > 0 {
		shown := failures
		if len(shown) > 5 {
			shown = shown[:5]
		}
		return result, fmt.Errorf("Bangumi 表情有 %d 个同步错误：%s", len(failures), strings.Join(shown, "；"))
	}
	return result, nil
}

func (s *BangumiSmileStore) validManifestAsset(manifest BangumiSmileManifest, code string) (BangumiSmileAsset, bool) {
	asset, path, ok := manifest.Resolve(s.config.Directory, code)
	if !ok {
		return BangumiSmileAsset{}, false
	}
	contentType, extension, err := inspectBangumiSmileFile(path)
	if err != nil || filepath.Ext(asset.File) != extension || asset.ContentType != contentType {
		return BangumiSmileAsset{}, false
	}
	return asset, true
}

func (s *BangumiSmileStore) findExistingAsset(definition bangumiSmileDefinition) (BangumiSmileAsset, bool) {
	baseName := smileAssetBaseName(definition.Code)
	for _, extension := range []string{".png", ".gif"} {
		filename := baseName + extension
		path := filepath.Join(s.config.Directory, filename)
		contentType, actualExtension, err := inspectBangumiSmileFile(path)
		if err != nil || actualExtension != extension {
			continue
		}
		return BangumiSmileAsset{
			Code: definition.Code, File: filename, ContentType: contentType,
			SourceURL: sourceURLForExtension(definition.SourceURLs, extension),
		}, true
	}
	return BangumiSmileAsset{}, false
}

func (s *BangumiSmileStore) download(ctx context.Context, client *http.Client, definition bangumiSmileDefinition) (BangumiSmileAsset, error) {
	var candidateErrors []string
	for _, sourceURL := range definition.SourceURLs {
		asset, status, err := s.downloadCandidate(ctx, client, definition.Code, sourceURL)
		if err == nil && status == http.StatusOK {
			return asset, nil
		}
		if status == http.StatusNotFound {
			continue
		}
		if err != nil {
			candidateErrors = append(candidateErrors, fmt.Sprintf("%s: %v", sourceURL, err))
			break
		}
	}
	if len(candidateErrors) > 0 {
		return BangumiSmileAsset{}, errors.New(strings.Join(candidateErrors, "；"))
	}
	return BangumiSmileAsset{}, errors.New("所有候选 PNG/GIF 地址均返回 404")
}

func (s *BangumiSmileStore) downloadCandidate(ctx context.Context, client *http.Client, code, sourceURL string) (BangumiSmileAsset, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return BangumiSmileAsset{}, 0, err
	}
	req.Header.Set("Accept", "image/png,image/gif;q=0.9,image/*;q=0.8")
	req.Header.Set("User-Agent", s.config.UserAgent)
	response, err := client.Do(req)
	if err != nil {
		return BangumiSmileAsset{}, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return BangumiSmileAsset{}, response.StatusCode, nil
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return BangumiSmileAsset{}, response.StatusCode,
			fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	temporary, err := os.CreateTemp(s.config.Directory, ".smile-*.tmp")
	if err != nil {
		return BangumiSmileAsset{}, response.StatusCode, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	written, copyErr := io.Copy(temporary, io.LimitReader(response.Body, maxSmileResponseBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return BangumiSmileAsset{}, response.StatusCode, copyErr
	}
	if closeErr != nil {
		return BangumiSmileAsset{}, response.StatusCode, closeErr
	}
	if written == 0 {
		return BangumiSmileAsset{}, response.StatusCode, errors.New("图片内容为空")
	}
	if written > maxSmileResponseBytes {
		return BangumiSmileAsset{}, response.StatusCode, fmt.Errorf("图片超过 %d MiB 限制", maxSmileResponseBytes>>20)
	}
	contentType, extension, err := inspectBangumiSmileFile(temporaryPath)
	if err != nil {
		return BangumiSmileAsset{}, response.StatusCode, err
	}
	filename := smileAssetBaseName(code) + extension
	destination := filepath.Join(s.config.Directory, filename)
	if err := replaceBangumiSmileFile(temporaryPath, destination); err != nil {
		return BangumiSmileAsset{}, response.StatusCode, err
	}
	return BangumiSmileAsset{Code: code, File: filename, ContentType: contentType, SourceURL: sourceURL}, response.StatusCode, nil
}

func (s *BangumiSmileStore) writeManifest(assets map[string]BangumiSmileAsset, complete bool, expected int) error {
	manifest := BangumiSmileManifest{
		Version: BangumiSmileManifestVersion, GeneratedAt: time.Now().UTC().Unix(),
		Complete: complete, ExpectedCount: expected, Assets: assets,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 Bangumi 表情清单: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(s.config.Directory, ".smile-manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("创建 Bangumi 表情临时清单: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入 Bangumi 表情清单: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭 Bangumi 表情清单: %w", err)
	}
	if err := replaceBangumiSmileFile(temporaryPath, filepath.Join(s.config.Directory, smileManifestFilename)); err != nil {
		return fmt.Errorf("保存 Bangumi 表情清单: %w", err)
	}
	return nil
}

func inspectBangumiSmileFile(path string) (string, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", "", err
	}
	if info.Size() <= 0 || info.Size() > maxSmileResponseBytes {
		return "", "", fmt.Errorf("Bangumi 表情文件大小无效: %d", info.Size())
	}
	header := make([]byte, 24)
	read, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", "", err
	}
	header = header[:read]
	if len(header) >= 10 && (string(header[:6]) == "GIF87a" || string(header[:6]) == "GIF89a") &&
		binary.LittleEndian.Uint16(header[6:8]) > 0 && binary.LittleEndian.Uint16(header[8:10]) > 0 {
		return "image/gif", ".gif", nil
	}
	pngSignature := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if len(header) >= 24 && string(header[:8]) == string(pngSignature) && string(header[12:16]) == "IHDR" &&
		binary.BigEndian.Uint32(header[16:20]) > 0 && binary.BigEndian.Uint32(header[20:24]) > 0 {
		return "image/png", ".png", nil
	}
	return "", "", errors.New("无效或不支持的 Bangumi GIF/PNG 表情图片")
}

func replaceBangumiSmileFile(source, destination string) error {
	renameErr := os.Rename(source, destination)
	if renameErr == nil {
		return nil
	}
	if _, err := os.Stat(destination); err != nil {
		return fmt.Errorf("移动表情文件到 %s: %w", destination, renameErr)
	}
	if err := os.Remove(destination); err != nil {
		return fmt.Errorf("替换旧表情文件 %s: %w", destination, err)
	}
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("移动表情文件到 %s: %w", destination, err)
	}
	return nil
}

func newBangumiSmileHTTPClient(settings system.NetworkSettings, timeout time.Duration) (*http.Client, error) {
	httpProxy, err := parseOptionalURL(settings.HTTPProxy)
	if err != nil {
		return nil, fmt.Errorf("HTTP 代理配置无效: %w", err)
	}
	httpsProxy, err := parseOptionalURL(settings.HTTPSProxy)
	if err != nil {
		return nil, fmt.Errorf("HTTPS 代理配置无效: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = timeout
	transport.Proxy = func(request *http.Request) (*url.URL, error) {
		if request.URL.Scheme == "https" {
			return httpsProxy, nil
		}
		return httpProxy, nil
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}

func bangumiSmileDefinitions(bangumiBaseURL, lainBaseURL string) []bangumiSmileDefinition {
	definitions := make([]bangumiSmileDefinition, 0, BangumiSmileAssetCount)
	for number := 24; number <= 125; number++ {
		code := fmt.Sprintf("(bgm%d)", number)
		definitions = append(definitions, bangumiSmileDefinition{
			Code:       code,
			SourceURLs: []string{fmt.Sprintf("%s/img/smiles/tv/%02d.gif", bangumiBaseURL, number-23)},
		})
	}
	for number := 200; number <= 238; number++ {
		code := fmt.Sprintf("(bgm%d)", number)
		definitions = append(definitions, bangumiSmileDefinition{
			Code:       code,
			SourceURLs: []string{fmt.Sprintf("%s/img/smiles/tv_vs/bgm_%d.png", bangumiBaseURL, number)},
		})
	}
	for number := 500; number <= 529; number++ {
		code := fmt.Sprintf("(bgm%d)", number)
		base := fmt.Sprintf("%s/img/smiles/tv_500/bgm_%d", bangumiBaseURL, number)
		definitions = append(definitions, bangumiSmileDefinition{Code: code, SourceURLs: []string{base + ".png", base + ".gif"}})
	}
	for number := 1; number <= 23; number++ {
		code := fmt.Sprintf("(bgm%02d)", number)
		base := fmt.Sprintf("%s/img/smiles/bgm/%02d", bangumiBaseURL, number)
		definitions = append(definitions, bangumiSmileDefinition{Code: code, SourceURLs: []string{base + ".png", base + ".gif"}})
	}
	for _, family := range []string{"musume", "blake"} {
		for number := 1; number <= 118; number++ {
			// Bangumi's official rich-text editor has no musume_97 or
			// musume_98 entries, and both upstream files return 404.
			if family == "musume" && (number == 97 || number == 98) {
				continue
			}
			code := fmt.Sprintf("(%s_%02d)", family, number)
			definitions = append(definitions, bangumiSmileDefinition{
				Code:       code,
				SourceURLs: []string{fmt.Sprintf("%s/img/smiles/%s/%s_%02d.gif", lainBaseURL, family, family, number)},
			})
		}
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Code < definitions[j].Code })
	return definitions
}

func smileAssetBaseName(code string) string {
	return strings.TrimSuffix(strings.TrimPrefix(code, "("), ")")
}

func sourceURLForExtension(sourceURLs []string, extension string) string {
	for _, sourceURL := range sourceURLs {
		parsed, err := url.Parse(sourceURL)
		if err == nil && strings.EqualFold(filepath.Ext(parsed.Path), extension) {
			return sourceURL
		}
	}
	if len(sourceURLs) > 0 {
		return sourceURLs[0]
	}
	return ""
}

func manifestIsComplete(manifest BangumiSmileManifest, assets map[string]BangumiSmileAsset, expected int) bool {
	if manifest.Version != BangumiSmileManifestVersion || !manifest.Complete || manifest.ExpectedCount != expected || len(manifest.Assets) != expected {
		return false
	}
	if len(assets) != expected {
		return false
	}
	for code, asset := range assets {
		if manifest.Assets[code] != asset {
			return false
		}
	}
	return true
}
