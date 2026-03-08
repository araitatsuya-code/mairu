package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultMaxRetries は一時エラー時の標準リトライ回数。
	DefaultMaxRetries = 3
	// DefaultRetryBackoff は指数バックオフの初期待機時間。
	DefaultRetryBackoff = time.Second
)

// EventKind はジョブイベント種別を表す。
type EventKind string

const (
	EventKindStarted        EventKind = "started"
	EventKindRetryScheduled EventKind = "retry_scheduled"
	EventKindSucceeded      EventKind = "succeeded"
	EventKindFailed         EventKind = "failed"
	EventKindSkipped        EventKind = "skipped"
	EventKindOverlapSkipped EventKind = "overlap_skipped"
)

// Result はジョブ実行結果の概要を表す。
type Result struct {
	Processed int
	Success   int
	Failed    int
	Skipped   bool
	Message   string
}

// Handler は 1 ジョブ分の実処理を表す。
type Handler func(context.Context) (Result, error)

// Job はスケジュール対象の 1 ジョブを表す。
type Job struct {
	ID           string
	Interval     time.Duration
	RunOnStart   bool
	MaxRetries   int
	RetryBackoff time.Duration
	Handler      Handler
}

// Event はジョブ実行時に通知されるイベントを表す。
type Event struct {
	At         time.Time
	JobID      string
	Kind       EventKind
	Attempt    int
	MaxRetries int
	Delay      time.Duration
	Result     Result
	Err        error
}

// Options は Service 生成時の設定を表す。
type Options struct {
	Jobs    []Job
	OnEvent func(Event)
	Now     func() time.Time
	Sleep   func(context.Context, time.Duration) error
}

// Service は goroutine ベースの定期実行スケジューラー。
type Service struct {
	jobs map[string]*jobRunner

	onEvent func(Event)
	now     func() time.Time
	sleep   func(context.Context, time.Duration) error

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	workers sync.WaitGroup
}

type jobRunner struct {
	job     Job
	trigger chan struct{}
	running atomic.Bool
}

// New はジョブ定義を検証して Service を生成する。
func New(options Options) (*Service, error) {
	if len(options.Jobs) == 0 {
		return nil, errors.New("scheduler の jobs は 1 件以上必要です")
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	sleep := options.Sleep
	if sleep == nil {
		sleep = defaultSleep
	}

	onEvent := options.OnEvent
	if onEvent == nil {
		onEvent = func(Event) {}
	}

	runners := make(map[string]*jobRunner, len(options.Jobs))
	for _, raw := range options.Jobs {
		job := raw
		job.ID = strings.TrimSpace(job.ID)
		if job.ID == "" {
			return nil, errors.New("scheduler job の ID は必須です")
		}
		if job.Handler == nil {
			return nil, fmt.Errorf("scheduler job %q の handler は必須です", job.ID)
		}
		if job.Interval <= 0 {
			return nil, fmt.Errorf("scheduler job %q の interval は 1ns 以上で指定してください", job.ID)
		}
		if _, exists := runners[job.ID]; exists {
			return nil, fmt.Errorf("scheduler job %q が重複しています", job.ID)
		}
		if job.MaxRetries < 0 {
			job.MaxRetries = 0
		}
		if job.RetryBackoff <= 0 {
			job.RetryBackoff = DefaultRetryBackoff
		}

		runners[job.ID] = &jobRunner{
			job:     job,
			trigger: make(chan struct{}, 1),
		}
	}

	return &Service{
		jobs:    runners,
		onEvent: onEvent,
		now:     now,
		sleep:   sleep,
	}, nil
}

// Start は各ジョブの定期実行 goroutine を開始する。
func (s *Service) Start(parent context.Context) error {
	if parent == nil {
		return errors.New("scheduler Start の context は必須です")
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("scheduler はすでに起動済みです")
	}

	ctx, cancel := context.WithCancel(parent)
	runners := make([]*jobRunner, 0, len(s.jobs))
	for _, runner := range s.jobs {
		runners = append(runners, runner)
	}
	s.workers.Add(len(runners))
	s.cancel = cancel
	s.started = true
	s.mu.Unlock()

	for _, runner := range runners {
		r := runner
		go func() {
			defer s.workers.Done()
			s.runWorker(ctx, r)
		}()
	}

	return nil
}

// Stop は実行中の scheduler を停止する。
func (s *Service) Stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	s.cancel = nil
	s.started = false
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.workers.Wait()
}

// Trigger は指定ジョブを即時実行キューへ積む。
func (s *Service) Trigger(jobID string) bool {
	runner, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok {
		return false
	}

	select {
	case runner.trigger <- struct{}{}:
	default:
	}
	return true
}

func (s *Service) runWorker(ctx context.Context, runner *jobRunner) {
	ticker := time.NewTicker(runner.job.Interval)
	defer ticker.Stop()

	var runWG sync.WaitGroup
	defer runWG.Wait()

	triggerRun := func() {
		if !runner.running.CompareAndSwap(false, true) {
			s.emit(Event{
				JobID:      runner.job.ID,
				Kind:       EventKindOverlapSkipped,
				MaxRetries: runner.job.MaxRetries,
			})
			return
		}

		runWG.Add(1)
		go func() {
			defer runWG.Done()
			defer runner.running.Store(false)
			s.runJob(ctx, runner.job)
		}()
	}

	if runner.job.RunOnStart {
		triggerRun()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			triggerRun()
		case <-runner.trigger:
			triggerRun()
		}
	}
}

func (s *Service) runJob(ctx context.Context, job Job) {
	attempt := 1
	for {
		s.emit(Event{
			JobID:      job.ID,
			Kind:       EventKindStarted,
			Attempt:    attempt,
			MaxRetries: job.MaxRetries,
		})

		result, err := job.Handler(ctx)
		if err == nil {
			kind := EventKindSucceeded
			if result.Skipped {
				kind = EventKindSkipped
			}
			s.emit(Event{
				JobID:      job.ID,
				Kind:       kind,
				Attempt:    attempt,
				MaxRetries: job.MaxRetries,
				Result:     result,
			})
			return
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			s.emit(Event{
				JobID:      job.ID,
				Kind:       EventKindFailed,
				Attempt:    attempt,
				MaxRetries: job.MaxRetries,
				Err:        err,
				Result:     result,
			})
			return
		}

		if attempt > job.MaxRetries || !IsRetryable(err) {
			s.emit(Event{
				JobID:      job.ID,
				Kind:       EventKindFailed,
				Attempt:    attempt,
				MaxRetries: job.MaxRetries,
				Err:        err,
				Result:     result,
			})
			return
		}

		retryDelay := job.RetryBackoff * time.Duration(1<<(attempt-1))
		s.emit(Event{
			JobID:      job.ID,
			Kind:       EventKindRetryScheduled,
			Attempt:    attempt,
			MaxRetries: job.MaxRetries,
			Delay:      retryDelay,
			Err:        err,
			Result:     result,
		})

		if sleepErr := s.sleep(ctx, retryDelay); sleepErr != nil {
			s.emit(Event{
				JobID:      job.ID,
				Kind:       EventKindFailed,
				Attempt:    attempt,
				MaxRetries: job.MaxRetries,
				Err:        sleepErr,
				Result:     result,
			})
			return
		}

		attempt++
	}
}

func (s *Service) emit(event Event) {
	event.At = s.now()
	s.onEvent(event)
}

// MarkRetryable は一時エラーとして扱いたいエラーをラップする。
func MarkRetryable(err error) error {
	if err == nil {
		return nil
	}
	return retryableError{err: err}
}

// IsRetryable はリトライ対象エラーかを判定する。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var wrapped retryableError
	if errors.As(err, &wrapped) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
		type temporary interface {
			Temporary() bool
		}
		var temporaryErr temporary
		if errors.As(err, &temporaryErr) {
			return temporaryErr.Temporary()
		}
	}

	type temporary interface {
		Temporary() bool
	}
	var temporaryErr temporary
	if errors.As(err, &temporaryErr) {
		return temporaryErr.Temporary()
	}

	return false
}

type retryableError struct {
	err error
}

func (e retryableError) Error() string {
	return e.err.Error()
}

func (e retryableError) Unwrap() error {
	return e.err
}

func (e retryableError) Temporary() bool {
	return true
}

func defaultSleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
