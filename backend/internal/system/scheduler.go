package system

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"bangumipipeline.local/server/internal/database"
)

var ErrExecutorNotFound = errors.New("scheduled task executor not found")

type Executor interface {
	Execute(context.Context) error
}

type ExecutorFunc func(context.Context) error

func (f ExecutorFunc) Execute(ctx context.Context) error {
	return f(ctx)
}

type Scheduler struct {
	service   *Service
	logger    *slog.Logger
	pollEvery time.Duration
	executors map[string]Executor

	mu      sync.Mutex
	ctx     context.Context
	running map[string]struct{}
}

func NewScheduler(service *Service, logger *slog.Logger, pollEvery time.Duration) *Scheduler {
	return &Scheduler{
		service: service, logger: logger, pollEvery: pollEvery,
		executors: make(map[string]Executor), running: make(map[string]struct{}),
	}
}

func (s *Scheduler) Register(key string, executor Executor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executors[key] = executor
}

func (s *Scheduler) Start(ctx context.Context) error {
	ctx = database.WithReadWorkload(ctx, database.ReadWorker)
	if err := s.service.PrepareScheduler(ctx); err != nil {
		return fmt.Errorf("prepare scheduler: %w", err)
	}
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	go s.loop(ctx)
	return nil
}

func (s *Scheduler) Trigger(key string) (ScheduledTask, error) {
	s.mu.Lock()
	executor, registered := s.executors[key]
	if !registered {
		s.mu.Unlock()
		return ScheduledTask{}, ErrExecutorNotFound
	}
	if _, exists := s.running[key]; exists {
		s.mu.Unlock()
		return ScheduledTask{}, ErrTaskAlreadyRunning
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	s.running[key] = struct{}{}
	s.mu.Unlock()

	task, err := s.service.MarkTaskStarted(ctx, key)
	if err != nil {
		s.clearRunning(key)
		return ScheduledTask{}, err
	}
	go s.execute(ctx, key, executor)
	return task, nil
}

func (s *Scheduler) loop(ctx context.Context) {
	s.dispatchDue(ctx)
	ticker := time.NewTicker(s.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dispatchDue(ctx)
		}
	}
}

func (s *Scheduler) dispatchDue(ctx context.Context) {
	keys, err := s.service.DueTaskKeys(ctx)
	if err != nil {
		s.logger.Error("list due scheduled tasks", "source", "scheduler", "error", err)
		return
	}
	for _, key := range keys {
		if _, err := s.Trigger(key); err != nil && !errors.Is(err, ErrTaskAlreadyRunning) {
			s.logger.Error("trigger scheduled task", "source", "scheduler", "task", key, "error", err)
		}
	}
}

func (s *Scheduler) execute(ctx context.Context, key string, executor Executor) {
	s.logger.Info("scheduled task started", "source", "scheduler", "task", key)
	runErr := executor.Execute(ctx)
	finishCtx, cancel := context.WithTimeout(database.WithReadWorkload(context.Background(), database.ReadWorker), 5*time.Second)
	defer cancel()
	if err := s.service.MarkTaskFinished(finishCtx, key, runErr); err != nil {
		s.logger.Error("persist scheduled task result", "source", "scheduler", "task", key, "error", err)
	}
	s.clearRunning(key)
	if runErr != nil {
		s.logger.Error("scheduled task failed", "source", "scheduler", "task", key, "error", runErr)
		return
	}
	s.logger.Info("scheduled task completed", "source", "scheduler", "task", key)
}

func (s *Scheduler) clearRunning(key string) {
	s.mu.Lock()
	delete(s.running, key)
	s.mu.Unlock()
}
