package applog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"bangumipipeline.local/server/internal/database"
)

const MaxInitialEntries = 1000

var validLevels = map[string]struct{}{
	"INFO": {}, "WARNING": {}, "ERROR": {},
}

type Entry struct {
	ID        int64          `json:"id"`
	Level     string         `json:"level"`
	Source    string         `json:"source"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields"`
	CreatedAt int64          `json:"createdAt"`
}

type Service struct {
	db database.Executor

	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan Entry
}

func NewService(db database.Executor) *Service {
	return &Service{db: db, subscribers: make(map[int]chan Entry)}
}

func NormalizeLevels(levels []string) ([]string, error) {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(levels))
	for _, level := range levels {
		level = strings.ToUpper(strings.TrimSpace(level))
		if level == "WARN" {
			level = "WARNING"
		}
		if level == "" {
			continue
		}
		if _, ok := validLevels[level]; !ok {
			return nil, fmt.Errorf("unsupported log level %q", level)
		}
		if _, ok := seen[level]; !ok {
			seen[level] = struct{}{}
			result = append(result, level)
		}
	}
	return result, nil
}

func (s *Service) Write(ctx context.Context, level, source, message string, fields map[string]any, createdAt time.Time) (Entry, error) {
	levels, err := NormalizeLevels([]string{level})
	if err != nil {
		return Entry{}, err
	}
	if len(levels) == 0 {
		levels = []string{"INFO"}
	}
	if strings.TrimSpace(source) == "" {
		source = "system"
	}
	if fields == nil {
		fields = map[string]any{}
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		encoded = []byte(`{"serializationError":"log fields could not be encoded"}`)
	}
	safeFields := make(map[string]any)
	_ = json.Unmarshal(encoded, &safeFields)
	entry := Entry{
		Level: levels[0], Source: source, Message: message, Fields: safeFields,
		CreatedAt: createdAt.UTC().UnixMilli(),
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO system_logs(level, source, message, fields_json, created_at)
VALUES (?, ?, ?, ?, ?)`, entry.Level, entry.Source, entry.Message, string(encoded), entry.CreatedAt)
	if err != nil {
		return Entry{}, err
	}
	entry.ID, err = result.LastInsertId()
	if err != nil {
		return Entry{}, err
	}
	s.publish(entry)
	return entry, nil
}

func (s *Service) List(ctx context.Context, levels []string, limit int) ([]Entry, error) {
	return s.query(ctx, levels, nil, 0, limit, false)
}

func (s *Service) ListAfter(ctx context.Context, levels []string, afterID int64, limit int) ([]Entry, error) {
	return s.query(ctx, levels, nil, afterID, limit, true)
}

func (s *Service) ListExcludingSources(ctx context.Context, levels []string, excludedSources []string, limit int) ([]Entry, error) {
	return s.query(ctx, levels, excludedSources, 0, limit, false)
}

func (s *Service) ListAfterExcludingSources(ctx context.Context, levels []string, excludedSources []string, afterID int64, limit int) ([]Entry, error) {
	return s.query(ctx, levels, excludedSources, afterID, limit, true)
}

func (s *Service) query(ctx context.Context, levels []string, excludedSources []string, afterID int64, limit int, ascending bool) ([]Entry, error) {
	normalized, err := NormalizeLevels(levels)
	if err != nil {
		return nil, err
	}
	excluded := normalizeSources(excludedSources)
	if limit < 1 || limit > MaxInitialEntries {
		limit = MaxInitialEntries
	}
	conditions := make([]string, 0, 3)
	args := make([]any, 0, len(normalized)+len(excluded)+2)
	if afterID > 0 {
		conditions = append(conditions, "id > ?")
		args = append(args, afterID)
	}
	if len(normalized) > 0 {
		placeholders := make([]string, len(normalized))
		for index, level := range normalized {
			placeholders[index] = "?"
			args = append(args, level)
		}
		conditions = append(conditions, "level IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(excluded) > 0 {
		placeholders := make([]string, len(excluded))
		for index, source := range excluded {
			placeholders[index] = "?"
			args = append(args, source)
		}
		conditions = append(conditions, "LOWER(source) NOT IN ("+strings.Join(placeholders, ",")+")")
	}
	query := "SELECT id, level, source, message, fields_json, created_at FROM system_logs"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	if ascending {
		query += " ORDER BY id ASC LIMIT ?"
	} else {
		query += " ORDER BY id DESC LIMIT ?"
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]Entry, 0)
	for rows.Next() {
		var entry Entry
		var fieldsJSON string
		if err := rows.Scan(&entry.ID, &entry.Level, &entry.Source, &entry.Message, &fieldsJSON, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entry.Fields = map[string]any{}
		_ = json.Unmarshal([]byte(fieldsJSON), &entry.Fields)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !ascending {
		for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
			entries[left], entries[right] = entries[right], entries[left]
		}
	}
	return entries, nil
}

func normalizeSources(sources []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(sources))
	for _, source := range sources {
		source = strings.ToLower(strings.TrimSpace(source))
		if source == "" {
			continue
		}
		if _, ok := seen[source]; !ok {
			seen[source] = struct{}{}
			result = append(result, source)
		}
	}
	return result
}

func (s *Service) Subscribe(buffer int) (<-chan Entry, func()) {
	if buffer < 1 {
		buffer = 64
	}
	s.mu.Lock()
	id := s.nextID
	s.nextID++
	channel := make(chan Entry, buffer)
	s.subscribers[id] = channel
	s.mu.Unlock()
	return channel, func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

func (s *Service) publish(entry Entry) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, subscriber := range s.subscribers {
		select {
		case subscriber <- entry:
		default:
			// A slow browser must not block task execution. Its next reconnect uses
			// ListAfter to fill any gap from SQLite.
		}
	}
}
