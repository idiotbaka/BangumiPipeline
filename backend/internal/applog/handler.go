package applog

import (
	"context"
	"log/slog"
	"time"
)

type Handler struct {
	base    slog.Handler
	service *Service
	attrs   []slog.Attr
	groups  []string
}

func NewHandler(base slog.Handler, service *Service) *Handler {
	return &Handler{base: base, service: service}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	baseErr := h.base.Handle(ctx, record)
	fields := make(map[string]any)
	for _, attr := range h.attrs {
		appendAttribute(fields, h.groups, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendAttribute(fields, h.groups, attr)
		return true
	})
	source := "system"
	if value, ok := fields["source"].(string); ok && value != "" {
		source = value
		delete(fields, "source")
	}
	level := "INFO"
	if record.Level >= slog.LevelError {
		level = "ERROR"
	} else if record.Level >= slog.LevelWarn {
		level = "WARNING"
	}
	createdAt := record.Time
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	_, storeErr := h.service.Write(ctx, level, source, record.Message, fields, createdAt)
	if baseErr != nil {
		return baseErr
	}
	return storeErr
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.base = h.base.WithAttrs(attrs)
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &clone
}

func (h *Handler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.base = h.base.WithGroup(name)
	clone.groups = append(append([]string(nil), h.groups...), name)
	return &clone
}

func appendAttribute(target map[string]any, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	current := target
	for _, group := range groups {
		nested, ok := current[group].(map[string]any)
		if !ok {
			nested = make(map[string]any)
			current[group] = nested
		}
		current = nested
	}
	if attr.Value.Kind() == slog.KindGroup {
		nested := make(map[string]any)
		for _, child := range attr.Value.Group() {
			appendAttribute(nested, nil, child)
		}
		current[attr.Key] = nested
		return
	}
	value := attr.Value.Any()
	if err, ok := value.(error); ok {
		value = err.Error()
	}
	current[attr.Key] = value
}
