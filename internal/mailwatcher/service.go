package mailwatcher

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"icloud-privacy-mail-v2/internal/config"
	"icloud-privacy-mail-v2/internal/domain"
	mailboxservice "icloud-privacy-mail-v2/internal/mailbox"
	"icloud-privacy-mail-v2/internal/protocol"
	"icloud-privacy-mail-v2/internal/store"
)

const (
	mailWatcherSyncTimeout = 90 * time.Second
	mailWatcherActiveTTL   = 20 * time.Minute
)

type Service struct {
	cfg             config.Config
	store           *store.Store
	mailbox         *mailboxservice.Service
	log             *slog.Logger
	wake            chan struct{}
	activeMu        sync.Mutex
	activeUntil     map[string]time.Time
	statusMu        sync.RWMutex
	status          Status
	readyWorkers    map[string]bool
	lastPublishedAt time.Time
}

// Status 是后台监听的真实运行快照，避免仅根据配置开关误报“运行中”。
type Status struct {
	Running              bool      `json:"running"`
	Enabled              bool      `json:"enabled"`
	GroupCount           int       `json:"group_count"`
	WorkerCount          int       `json:"worker_count"`
	ConnectedWorkerCount int       `json:"connected_worker_count"`
	SyncedMessages       int       `json:"synced_messages"`
	IdleEvents           int       `json:"idle_events"`
	StartedAt            time.Time `json:"started_at,omitempty"`
	LastCycleAt          time.Time `json:"last_cycle_at,omitempty"`
	LastSuccessAt        time.Time `json:"last_success_at,omitempty"`
	LastIdleConnectedAt  time.Time `json:"last_idle_connected_at,omitempty"`
	LastIdleEventAt      time.Time `json:"last_idle_event_at,omitempty"`
	LastErrorAt          time.Time `json:"last_error_at,omitempty"`
	LastError            string    `json:"last_error,omitempty"`
	LastIdleErrorAt      time.Time `json:"last_idle_error_at,omitempty"`
	LastIdleError        string    `json:"last_idle_error,omitempty"`
}

type watchGroup struct {
	key       string
	session   domain.ICloudSession
	state     domain.LoginState
	mailboxes []domain.Mailbox
	signature string
}

type idleWorker struct {
	cancel    context.CancelFunc
	signature string
}

func NewService(cfg config.Config, state *store.Store, mailbox *mailboxservice.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	service := &Service{
		cfg:          cfg,
		store:        state,
		mailbox:      mailbox,
		log:          logger,
		wake:         make(chan struct{}, 1),
		activeUntil:  make(map[string]time.Time),
		readyWorkers: make(map[string]bool),
	}
	if state != nil {
		var persisted Status
		if found, err := state.LoadRuntimeState("mailwatcher", &persisted); err == nil && found {
			persisted.Running = false
			persisted.WorkerCount = 0
			persisted.ConnectedWorkerCount = 0
			service.status = persisted
		}
	}
	return service
}

func (s *Service) Run(ctx context.Context) {
	interval := time.Duration(s.cfg.MailWatcherPollMS) * time.Millisecond
	if interval < time.Second {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	workers := make(map[string]idleWorker)
	defer stopIdleWorkers(workers)
	defer s.resetReadyWorkers()
	s.log.Info("后台邮件监听已启动", "轮询间隔", interval)
	s.updateStatus(func(status *Status) {
		status.Running = true
		status.StartedAt = time.Now()
	})
	defer s.updateStatus(func(status *Status) {
		status.Running = false
		status.WorkerCount = 0
		status.ConnectedWorkerCount = 0
	})

	started := false
	cycle := func() {
		enabled := s.cfg.MailWatcherEnabled && s.store.Settings().EnableMailWatcher
		groups := s.groups()
		s.updateStatus(func(status *Status) {
			status.Enabled = enabled
			status.GroupCount = len(groups)
			status.LastCycleAt = time.Now()
		})
		if !enabled {
			if started {
				stopIdleWorkers(workers)
				started = false
			}
			s.resetReadyWorkers()
			s.updateStatus(func(status *Status) { status.WorkerCount = 0 })
			return
		}
		initial := !started
		synced, syncErr := s.syncRound(ctx, groups, initial)
		s.recordSyncResult(synced, syncErr)
		if !started {
			started = true
		}
		s.ensureIdleWorkers(ctx, workers, groups)
		s.updateStatus(func(status *Status) { status.WorkerCount = len(workers) })
	}
	cycle()
	for {
		select {
		case <-ctx.Done():
			s.log.Info("后台邮件监听已停止")
			return
		case <-s.wake:
			cycle()
		case <-ticker.C:
			cycle()
		}
	}
}

// Wake 把主动取码的邮箱提升到本轮同步前面，并立即唤醒监听器。
func (s *Service) Wake(mailboxID string) {
	mailboxID = strings.TrimSpace(mailboxID)
	if mailboxID != "" {
		s.activeMu.Lock()
		s.activeUntil[mailboxID] = time.Now().Add(mailWatcherActiveTTL)
		s.activeMu.Unlock()
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) syncRound(ctx context.Context, groups []watchGroup, initial bool) (int, error) {
	if len(groups) == 0 {
		return 0, nil
	}
	after := time.Time{}
	limit := s.cfg.MailWatcherFetchLimit
	if initial {
		limit = s.cfg.MailWatcherInitialFetchLimit
		if s.cfg.MailWatcherLookbackHours > 0 {
			after = time.Now().Add(-time.Duration(s.cfg.MailWatcherLookbackHours) * time.Hour)
		}
	}
	total := 0
	var lastErr error
	for _, group := range groups {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
		syncCtx, cancel := context.WithTimeout(ctx, mailWatcherSyncTimeout)
		count, err := s.mailbox.SyncMailboxBatch(syncCtx, group.mailboxes, after, "OpenAI", limit)
		cancel()
		total += count
		if err != nil && ctx.Err() == nil {
			lastErr = err
			s.log.Warn("后台批量同步邮箱失败", "账号", group.session.AppleID, "邮箱数", len(group.mailboxes), "首次同步", initial, "错误", err)
		}
	}
	return total, lastErr
}

func (s *Service) ensureIdleWorkers(ctx context.Context, workers map[string]idleWorker, groups []watchGroup) {
	seen := make(map[string]bool, len(groups))
	for _, group := range groups {
		seen[group.key] = true
		if worker, ok := workers[group.key]; ok && worker.signature == group.signature {
			continue
		}
		if worker, ok := workers[group.key]; ok {
			worker.cancel()
			s.markWorkerReady(group.key, false)
			delete(workers, group.key)
		}
		workerCtx, cancel := context.WithCancel(ctx)
		workers[group.key] = idleWorker{cancel: cancel, signature: group.signature}
		go s.runIdleWorker(workerCtx, group)
	}
	for key, worker := range workers {
		if seen[key] {
			continue
		}
		worker.cancel()
		s.markWorkerReady(key, false)
		delete(workers, key)
	}
}

func (s *Service) runIdleWorker(ctx context.Context, group watchGroup) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := protocol.WatchICloudIMAPExists(ctx, group.state, func() {
			s.markWorkerReady(group.key, true)
			s.updateStatus(func(status *Status) {
				status.LastIdleConnectedAt = time.Now()
				status.LastIdleError = ""
			})
		}, func() {
			if ctx.Err() != nil {
				return
			}
			s.updateStatus(func(status *Status) {
				status.IdleEvents++
				status.LastIdleEventAt = time.Now()
			})
			syncCtx, cancel := context.WithTimeout(ctx, mailWatcherSyncTimeout)
			count, syncErr := s.mailbox.SyncMailboxBatch(syncCtx, group.mailboxes, time.Time{}, "OpenAI", s.cfg.MailWatcherFetchLimit)
			cancel()
			s.recordSyncResult(count, syncErr)
			if syncErr != nil && ctx.Err() == nil {
				s.log.Warn("IMAP IDLE 触发同步失败", "账号", group.session.AppleID, "邮箱数", len(group.mailboxes), "错误", syncErr)
			}
		})
		s.markWorkerReady(group.key, false)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			// 收到 EXISTS 后连接会退出当前 IDLE，立即重新建立监听，不进入故障退避。
			backoff = time.Second
			continue
		}
		s.recordWatcherError(err)
		s.log.Warn("IMAP IDLE 已断开，准备重连", "账号", group.session.AppleID, "邮箱数", len(group.mailboxes), "等待", backoff, "错误", err)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

// Snapshot 返回并发安全的后台监听运行快照。
func (s *Service) Snapshot() Status {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.status
}

func (s *Service) updateStatus(update func(*Status)) {
	s.statusMu.Lock()
	before := materialStatus(s.status)
	update(&s.status)
	after := materialStatus(s.status)
	now := time.Now()
	shouldPublish := before != after || s.lastPublishedAt.IsZero() || now.Sub(s.lastPublishedAt) >= 30*time.Second
	snapshot := s.status
	if shouldPublish {
		s.lastPublishedAt = now
	}
	s.statusMu.Unlock()
	if shouldPublish && s.store != nil {
		_ = s.store.SaveRuntimeState("mailwatcher", snapshot, true)
	}
}

type statusMaterial struct {
	Running              bool
	Enabled              bool
	GroupCount           int
	WorkerCount          int
	ConnectedWorkerCount int
	SyncedMessages       int
	IdleEvents           int
	LastError            string
	LastIdleError        string
}

func materialStatus(status Status) statusMaterial {
	return statusMaterial{
		Running: status.Running, Enabled: status.Enabled, GroupCount: status.GroupCount,
		WorkerCount: status.WorkerCount, ConnectedWorkerCount: status.ConnectedWorkerCount,
		SyncedMessages: status.SyncedMessages, IdleEvents: status.IdleEvents,
		LastError: status.LastError, LastIdleError: status.LastIdleError,
	}
}

func (s *Service) recordSyncResult(count int, err error) {
	now := time.Now()
	s.updateStatus(func(status *Status) {
		if count > 0 {
			status.SyncedMessages += count
		}
		if err != nil {
			status.LastError = err.Error()
			status.LastErrorAt = now
			return
		}
		status.LastSuccessAt = now
		status.LastError = ""
	})
}

func (s *Service) recordWatcherError(err error) {
	if err == nil {
		return
	}
	s.updateStatus(func(status *Status) {
		status.LastIdleError = err.Error()
		status.LastIdleErrorAt = time.Now()
	})
}

func (s *Service) markWorkerReady(key string, ready bool) {
	s.updateStatus(func(status *Status) {
		if ready {
			s.readyWorkers[key] = true
		} else {
			delete(s.readyWorkers, key)
		}
		status.ConnectedWorkerCount = len(s.readyWorkers)
	})
}

func (s *Service) resetReadyWorkers() {
	s.updateStatus(func(status *Status) {
		s.readyWorkers = make(map[string]bool)
		status.ConnectedWorkerCount = 0
	})
}

func (s *Service) groups() []watchGroup {
	active := s.activeMailboxIDs(time.Now())
	type bucket struct {
		session   domain.ICloudSession
		state     domain.LoginState
		mailboxes []domain.Mailbox
	}
	buckets := make(map[string]*bucket)
	for _, mailbox := range s.store.AllMailboxes() {
		if !mailbox.APIActive || !mailbox.ICloudActive || mailbox.Status == domain.StatusDisabled {
			continue
		}
		session, ok := s.store.ICloudSessionByAccountID(mailbox.AccountID)
		if !ok {
			continue
		}
		imapState, ok := protocol.LoginStateForKind(session, domain.LoginStateICloudIMAP)
		if !ok || strings.TrimSpace(imapState.IMAPAppPassword) == "" {
			continue
		}
		key := firstNonEmpty(session.AccountID, mailbox.AccountID, mailbox.OwnerID, "__imap__")
		item := buckets[key]
		if item == nil {
			item = &bucket{session: session, state: imapState}
			buckets[key] = item
		}
		item.mailboxes = append(item.mailboxes, mailbox)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := make([]watchGroup, 0, len(keys))
	for _, key := range keys {
		item := buckets[key]
		sort.Slice(item.mailboxes, func(i, j int) bool {
			leftActive := active[item.mailboxes[i].ID]
			rightActive := active[item.mailboxes[j].ID]
			if leftActive != rightActive {
				return leftActive
			}
			return item.mailboxes[i].Email < item.mailboxes[j].Email
		})
		groups = append(groups, watchGroup{
			key:       key,
			session:   item.session,
			state:     item.state,
			mailboxes: item.mailboxes,
			signature: groupSignature(item.state, item.mailboxes),
		})
	}
	return groups
}

func (s *Service) activeMailboxIDs(now time.Time) map[string]bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	active := make(map[string]bool)
	for mailboxID, until := range s.activeUntil {
		if until.After(now) {
			active[mailboxID] = true
			continue
		}
		delete(s.activeUntil, mailboxID)
	}
	return active
}

func groupSignature(state domain.LoginState, mailboxes []domain.Mailbox) string {
	parts := []string{state.IMAPEmail, state.IMAPUsername, state.IMAPHost, fmt.Sprint(state.IMAPPort), state.IMAPAppPassword}
	for _, mailbox := range mailboxes {
		parts = append(parts, mailbox.ID, mailbox.Email)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "|"))))
}

func stopIdleWorkers(workers map[string]idleWorker) {
	for key, worker := range workers {
		worker.cancel()
		delete(workers, key)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
