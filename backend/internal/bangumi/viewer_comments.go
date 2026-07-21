package bangumi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ViewerEpisodeComments struct {
	EpisodeID    int64                   `json:"episodeId"`
	SyncStatus   string                  `json:"syncStatus"`
	FetchedAt    *int64                  `json:"fetchedAt"`
	CommentCount int                     `json:"commentCount"`
	TotalCount   int                     `json:"totalCount"`
	Comments     []*ViewerEpisodeComment `json:"comments"`
}

type ViewerEpisodeComment struct {
	CommentID       int64                     `json:"commentId"`
	ParentCommentID int64                     `json:"parentCommentId"`
	CreatedAt       int64                     `json:"createdAt"`
	Content         string                    `json:"content"`
	State           int                       `json:"state"`
	User            *ViewerEpisodeCommentUser `json:"user"`
	Replies         []*ViewerEpisodeComment   `json:"replies"`
	sortOrder       int
}

type ViewerEpisodeCommentUser struct {
	UserID    int64  `json:"userId"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
	Group     int    `json:"group"`
	Sign      string `json:"sign"`
}

// ViewerEpisodeComments resolves a completed media product to its stable
// Bangumi episode ID before reading comments. Clients cannot use this endpoint
// to enumerate unrelated episode comments by supplying an arbitrary episode ID.
func (c *Catalog) ViewerEpisodeComments(ctx context.Context, bangumiID, mediaID int64) (ViewerEpisodeComments, error) {
	result := ViewerEpisodeComments{Comments: make([]*ViewerEpisodeComment, 0)}
	var status string
	var fetchedAt sql.NullInt64
	err := c.db.QueryRowContext(ctx, `
SELECT ae.episode_id, COALESCE(sync.status, ''), sync.last_fetched_at
FROM media_jobs mj
`+mediaEpisodeCommentJoin+`
LEFT JOIN bangumi_episode_comment_sync sync ON sync.episode_id = ae.episode_id
WHERE mj.id = ? AND mj.bangumi_id = ?
  AND mj.status = 'completed' AND mj.output_path != ''`, mediaID, bangumiID).Scan(
		&result.EpisodeID, &status, &fetchedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ViewerEpisodeComments{}, ErrAnimeNotFound
	}
	if err != nil {
		return ViewerEpisodeComments{}, err
	}
	result.SyncStatus = status
	if result.SyncStatus == "" {
		result.SyncStatus = "not_started"
	}
	if fetchedAt.Valid {
		value := fetchedAt.Int64
		result.FetchedAt = &value
	}

	rows, err := c.db.QueryContext(ctx, `
SELECT comment_id, parent_comment_id, source_created_at, content, state, sort_order,
       comments.user_id, username, nickname, COALESCE(avatars.file_name, ''),
       user_group, user_sign
FROM bangumi_episode_comments comments
LEFT JOIN bangumi_comment_user_avatars avatars
       ON avatars.user_id = comments.user_id AND avatars.status = 'downloaded'
WHERE bangumi_id = ? AND episode_id = ?
  AND NOT EXISTS (
      SELECT 1
      FROM viewer_comment_username_filters filters
      WHERE filters.username = comments.username COLLATE BINARY
  )
ORDER BY source_created_at DESC, sort_order DESC, comment_id DESC`, bangumiID, result.EpisodeID)
	if err != nil {
		return ViewerEpisodeComments{}, err
	}
	nodes := make([]*ViewerEpisodeComment, 0)
	for rows.Next() {
		var comment ViewerEpisodeComment
		var user ViewerEpisodeCommentUser
		var avatarFileName string
		if err := rows.Scan(
			&comment.CommentID, &comment.ParentCommentID, &comment.CreatedAt,
			&comment.Content, &comment.State, &comment.sortOrder,
			&user.UserID, &user.Username, &user.Nickname,
			&avatarFileName, &user.Group, &user.Sign,
		); err != nil {
			rows.Close()
			return ViewerEpisodeComments{}, err
		}
		comment.Replies = make([]*ViewerEpisodeComment, 0)
		if avatarFileName == commentAvatarFileName(user.UserID) {
			user.AvatarURL = "/api/bangumi-comment-avatars/" + strconv.FormatInt(user.UserID, 10)
		}
		if user.UserID > 0 || strings.TrimSpace(user.Username) != "" || strings.TrimSpace(user.Nickname) != "" {
			comment.User = &user
		}
		nodes = append(nodes, &comment)
	}
	if err := rows.Close(); err != nil {
		return ViewerEpisodeComments{}, err
	}
	if err := rows.Err(); err != nil {
		return ViewerEpisodeComments{}, err
	}
	result.TotalCount = len(nodes)
	result.Comments = buildViewerCommentTree(nodes)
	result.CommentCount = len(result.Comments)
	return result, nil
}

func (c *Catalog) CommentAvatarPath(ctx context.Context, userID int64) (string, error) {
	if userID <= 0 {
		return "", ErrAnimeNotFound
	}
	var fileName string
	err := c.db.QueryRowContext(ctx, `
SELECT file_name
FROM bangumi_comment_user_avatars
WHERE user_id = ? AND status = 'downloaded'`, userID).Scan(&fileName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAnimeNotFound
	}
	if err != nil {
		return "", err
	}
	expected := commentAvatarFileName(userID)
	if fileName != expected || filepath.Base(fileName) != fileName {
		return "", fmt.Errorf("用户 #%d 的评论头像缓存文件名无效", userID)
	}
	return filepath.Join(c.commentAvatarDir, fileName), nil
}

func buildViewerCommentTree(nodes []*ViewerEpisodeComment) []*ViewerEpisodeComment {
	byID := make(map[int64]*ViewerEpisodeComment, len(nodes))
	for _, node := range nodes {
		byID[node.CommentID] = node
		node.Replies = make([]*ViewerEpisodeComment, 0)
	}
	roots := make([]*ViewerEpisodeComment, 0)
	for _, node := range nodes {
		parent, exists := byID[node.ParentCommentID]
		if !exists || node.ParentCommentID <= 0 || viewerCommentParentCycle(node.CommentID, node.ParentCommentID, byID) {
			roots = append(roots, node)
			continue
		}
		parent.Replies = append(parent.Replies, node)
	}
	sortViewerCommentSiblings(roots)
	return roots
}

func viewerCommentParentCycle(commentID, parentID int64, byID map[int64]*ViewerEpisodeComment) bool {
	seen := make(map[int64]struct{})
	for current := parentID; current > 0; {
		if current == commentID {
			return true
		}
		if _, duplicate := seen[current]; duplicate {
			return true
		}
		seen[current] = struct{}{}
		parent, exists := byID[current]
		if !exists {
			return false
		}
		current = parent.ParentCommentID
	}
	return false
}

func sortViewerCommentSiblings(comments []*ViewerEpisodeComment) {
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].CreatedAt != comments[j].CreatedAt {
			return comments[i].CreatedAt > comments[j].CreatedAt
		}
		if comments[i].sortOrder != comments[j].sortOrder {
			return comments[i].sortOrder > comments[j].sortOrder
		}
		return comments[i].CommentID > comments[j].CommentID
	})
	for _, comment := range comments {
		sortViewerCommentSiblings(comment.Replies)
	}
}
