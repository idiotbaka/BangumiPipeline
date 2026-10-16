package download

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"html"
	"net/url"
	"strings"
	"unicode"
)

var (
	ErrMagnetRequired = errors.New("download magnet URL required")
	ErrInvalidMagnet  = errors.New("invalid download magnet URL")
)

// normalizeMagnet preserves trackers and display names, and canonicalizes BTIH
// to the hexadecimal form used by qBittorrent's torrents/info response.
func normalizeMagnet(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 16384 || strings.ContainsFunc(value, unicode.IsSpace) {
		return "", "", ErrInvalidMagnet
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "magnet") || parsed.Host != "" || parsed.Path != "" || parsed.Opaque != "" || parsed.Fragment != "" {
		return "", "", ErrInvalidMagnet
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", "", ErrInvalidMagnet
	}
	hash := ""
	var topics []string
	for key, values := range query {
		if !strings.EqualFold(key, "xt") {
			continue
		}
		delete(query, key)
		for _, topic := range values {
			if !strings.HasPrefix(strings.ToLower(topic), "urn:btih:") {
				topics = append(topics, topic)
				continue
			}
			encoded := topic[len("urn:btih:"):]
			var decoded []byte
			if len(encoded) == 40 {
				decoded, err = hex.DecodeString(encoded)
			} else if len(encoded) == 32 {
				decoded, err = base32.StdEncoding.DecodeString(strings.ToUpper(encoded))
			} else {
				return "", "", ErrInvalidMagnet
			}
			if err != nil || len(decoded) != 20 {
				return "", "", ErrInvalidMagnet
			}
			normalized := hex.EncodeToString(decoded)
			if hash != "" && hash != normalized {
				return "", "", ErrInvalidMagnet
			}
			hash = normalized
		}
	}
	if hash == "" {
		return "", "", ErrInvalidMagnet
	}
	query["xt"] = append(topics, "urn:btih:"+hash)
	return "magnet:?" + query.Encode(), hash, nil
}

func findMagnet(sources []string, description string) string {
	// Prefer complete magnets (and their trackers) over generated bare BTIHs.
	for _, source := range sources {
		if magnet, _, err := normalizeMagnet(source); err == nil {
			return magnet
		}
	}
	description = html.UnescapeString(description)
	for _, word := range strings.FieldsFunc(description, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`<>"'`, r)
	}) {
		if magnet, _, err := normalizeMagnet(word); err == nil {
			return magnet
		}
	}
	for _, source := range sources {
		if magnet, _, err := normalizeMagnet(normalizeDownloadSourceURL(source)); err == nil {
			return magnet
		}
	}
	return ""
}

// RetryFailedJobWithMagnet only changes the selected job. An empty input first
// attempts local discovery; ErrMagnetRequired has no database or qBit effects.
func (s *Service) RetryFailedJobWithMagnet(ctx context.Context, jobID int64, input string) (RetryResult, error) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	job, err := s.failedJob(ctx, jobID)
	if err != nil {
		return RetryResult{}, err
	}
	if _, err := s.cancelableJob(ctx, jobID); err != nil {
		if errors.Is(err, ErrCancelNotAllowed) {
			return RetryResult{}, ErrRetryNotAllowed
		}
		return RetryResult{}, err
	}
	var saved, source, enclosure, torrentURL, link, guid, description string
	err = s.db.QueryRowContext(ctx, `
SELECT dj.retry_magnet_url, dj.source_url, si.enclosure_url, si.torrent_url, si.link, si.guid, si.description
FROM download_jobs dj JOIN subscription_items si ON si.id = dj.subscription_item_id
WHERE dj.id = ?`, jobID).Scan(&saved, &source, &enclosure, &torrentURL, &link, &guid, &description)
	if err != nil {
		return RetryResult{}, err
	}
	magnet := ""
	if strings.TrimSpace(input) != "" {
		magnet, _, err = normalizeMagnet(input)
		if err != nil {
			return RetryResult{}, err
		}
	} else {
		magnet = findMagnet([]string{saved, source, enclosure, torrentURL, link, guid}, description)
		if magnet == "" {
			// A previously observed qBit hash is also a verified resource identity.
			magnet, _, _ = normalizeMagnet("magnet:?xt=urn:btih:" + job.QBitHash)
		}
		if magnet == "" {
			return RetryResult{}, ErrMagnetRequired
		}
	}
	_, hash, _ := normalizeMagnet(magnet)
	settings, err := s.settings.GetDownloadSettings(ctx)
	if err != nil {
		return RetryResult{}, err
	}
	client, err := newQBitClient(settings)
	if err != nil {
		return RetryResult{}, err
	}
	if err := client.login(ctx, settings.Username, settings.Password); err != nil {
		return RetryResult{}, errors.Join(ErrQBitUnavailable, err)
	}
	torrents, err := client.torrents(ctx)
	if err != nil {
		return RetryResult{}, errors.Join(ErrQBitUnavailable, err)
	}
	job.QBitSavePath = s.qBitSavePath(job.SavePath, settings.QBitDownloadDir)
	torrent, exists := matchTorrent(job, torrents, torrentsByHash(torrents))
	if exists {
		if statusFromTorrent(torrent) == StatusFailed {
			// Keep downloaded data so qBit can resume after recreating the task.
			if err := client.deleteTorrents(ctx, []string{torrent.Hash}, false); err != nil {
				return RetryResult{}, errors.Join(ErrQBitUnavailable, err)
			}
		} else if !strings.EqualFold(torrent.Hash, hash) {
			// A different healthy resource must not share this job's directory.
			return RetryResult{}, ErrRetryNotAllowed
		}
	}
	updated, err := s.resetFailedJobSource(ctx, jobID, magnet)
	if err != nil {
		return RetryResult{}, err
	}
	s.logger.Info("下载任务已改用磁力链接并重置为待下载", "source", "download", "job_id", jobID, "info_hash", hash)
	return RetryResult{Job: updated, Action: "magnet_reset"}, nil
}

func (s *Service) resetFailedJobSource(ctx context.Context, jobID int64, magnet string) (Job, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE download_jobs
SET status = 'pending', retry_magnet_url = ?, source_url = ?,
    qbit_hash = '', qbit_name = '', progress = 0, total_size = 0, downloaded_size = 0,
    download_speed = 0, error_message = '', started_at = NULL, completed_at = NULL, failed_at = NULL,
    updated_at = ?
WHERE id = ? AND status = 'failed'
  AND COALESCE(NULLIF(source_type, ''), 'download') = 'download'
  AND EXISTS (SELECT 1 FROM subscription_items si WHERE si.id = download_jobs.subscription_item_id AND si.binding_status = 'bound')
  AND NOT EXISTS (SELECT 1 FROM media_jobs mj WHERE mj.download_job_id = download_jobs.id)`,
		magnet, magnet, s.now().UTC().Unix(), jobID)
	if err != nil {
		return Job{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return Job{}, err
	}
	if count == 0 {
		return Job{}, ErrRetryNotAllowed
	}
	return s.jobByID(ctx, jobID)
}

// Adopting a manually added torrent with another save path would break media
// processing and cleanup. Match its identity, but refuse the unsafe path.
func matchMagnetTorrent(torrents []qBitTorrent, hash, savePath, qBitSavePath string) (qBitTorrent, bool, error) {
	for _, torrent := range torrents {
		if !strings.EqualFold(torrent.Hash, hash) {
			continue
		}
		if !sameDownloadPath(torrent.SavePath, savePath) && !sameDownloadPath(torrent.SavePath, qBitSavePath) {
			return qBitTorrent{}, false, errors.New("相同磁力链接的任务已存在，但保存目录不同；请先在 qBittorrent 中移除该任务（保留文件），再重试")
		}
		return torrent, true, nil
	}
	return qBitTorrent{}, false, nil
}
