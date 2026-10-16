package opskip

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Bump this version when matching, thresholds, analysis windows or silence
// correction change. Fingerprint format changes also invalidate group results.
const groupAnalysisVersion = "opening-v1/" + chromaprintVersion

type episodeInput struct {
	MediaID     int64
	Number      string
	Path        string
	Size        int64
	MTimeNS     int64
	DurationMS  int64
	CompletedAt int64
}

func groupInputSignature(ctx context.Context, group analysisGroup) (string, error) {
	inputs := make([]episodeInput, 0, len(group.episodes))
	for _, episode := range group.episodes {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		info, err := os.Stat(episode.OutputPath)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("成品视频不是普通文件: %s", episode.OutputPath)
		}
		inputs = append(inputs, episodeInput{
			MediaID: episode.MediaJobID, Number: episode.EpisodeNumber, Path: episode.OutputPath,
			Size: info.Size(), MTimeNS: info.ModTime().UTC().UnixNano(),
			DurationMS: episode.TotalDurationMS, CompletedAt: episode.CompletedAt,
		})
	}
	// The signature records exact members, not merely the episode count.
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].MediaID < inputs[j].MediaID })
	payload, err := json.Marshal(struct {
		Version string
		Group   groupKey
		Inputs  []episodeInput
	}{groupAnalysisVersion, group.key, inputs})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(payload)), nil
}

func completeGroupResults(group analysisGroup) bool {
	for _, episode := range group.episodes {
		if episode.AnalyzedGroupSize != len(group.episodes) {
			return false
		}
		switch episode.SegmentStatus {
		case StatusDetected:
			if !validOpeningRange(episode.SegmentStart, episode.SegmentEnd) {
				return false
			}
		case StatusNotFound:
		default:
			return false
		}
	}
	return true
}

func groupAnalysisReason(group analysisGroup, signature string, snapshotErr error) string {
	switch {
	case snapshotErr != nil:
		return "media_unavailable"
	case group.signature == "":
		return "no_checkpoint"
	case group.version != groupAnalysisVersion:
		return "analysis_version_changed"
	case group.signature != signature:
		return "inputs_changed"
	case !completeGroupResults(group):
		return "incomplete_results"
	default:
		return ""
	}
}

func (s *Service) invalidateGroupAnalysis(ctx context.Context, key groupKey) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM media_op_group_analysis WHERE bangumi_id = ? AND season_number = ?`, key.BangumiID, key.SeasonNumber)
	return err
}

func (s *Service) completeGroupAnalysis(ctx context.Context, group analysisGroup, signature string, result groupSummary) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// A silence failure retains the existing fingerprint-based playback fallback,
	// but must remain retryable rather than freezing an incomplete correction.
	if signature == "" || result.failed > 0 || result.silenceFailed > 0 || result.detected+result.notFound != len(group.episodes) {
		return false, nil
	}
	// Media processing or storage moves can change the group while ffmpeg runs.
	// Re-read membership and stat the files again before persisting completion.
	current, err := s.completedEpisodeGroups(ctx, &group.key)
	if err != nil {
		return false, err
	}
	if len(current) != 1 || len(current[0].episodes) < minimumGroupEpisodes {
		return false, nil
	}
	after, snapshotErr := groupInputSignature(ctx, current[0])
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if snapshotErr != nil || after != signature || !completeGroupResults(current[0]) {
		s.logger.Info("片头识别期间分组或文件发生变化，等待下轮重新分析", "source", "opskip",
			"bangumi_id", group.key.BangumiID, "season", group.key.SeasonNumber)
		return false, nil
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO media_op_group_analysis(bangumi_id, season_number, input_signature, analysis_version, episode_count, completed_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(bangumi_id, season_number) DO UPDATE SET
    input_signature = excluded.input_signature,
    analysis_version = excluded.analysis_version,
    episode_count = excluded.episode_count,
    completed_at = excluded.completed_at`, group.key.BangumiID, group.key.SeasonNumber,
		signature, groupAnalysisVersion, len(group.episodes), s.now().UTC().Unix())
	return err == nil, err
}
