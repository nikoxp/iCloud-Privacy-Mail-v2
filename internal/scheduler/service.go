package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"icloud-privacy-mail-v2/internal/domain"
	mailboxservice "icloud-privacy-mail-v2/internal/mailbox"
	"icloud-privacy-mail-v2/internal/store"
)

const maxEvents = 200

type Config struct {
	AccountIDs          []string `json:"account_ids"`
	Label               string   `json:"label"`
	Note                string   `json:"note"`
	CreateChannel       string   `json:"create_channel"`
	IntervalMinutes     int      `json:"interval_minutes"`
	IntervalSeconds     int      `json:"interval_seconds,omitempty"`
	RoundIntervalSecond int      `json:"round_interval_seconds"`
}

type Event struct {
	ID        int64     `json:"id"`
	At        time.Time `json:"at"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	AccountID string    `json:"account_id,omitempty"`
	MailboxID string    `json:"mailbox_id,omitempty"`
	Email     string    `json:"email,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type State struct {
	Running             bool      `json:"running"`
	Status              string    `json:"status"`
	AccountIDs          []string  `json:"account_ids"`
	Label               string    `json:"label"`
	Note                string    `json:"note"`
	CreateChannel       string    `json:"create_channel"`
	IntervalSeconds     int       `json:"interval_seconds"`
	RoundIntervalSecond int       `json:"round_interval_seconds"`
	BatchIndex          int       `json:"batch_index"`
	Success             int       `json:"success"`
	Failed              int       `json:"failed"`
	StartedAt           time.Time `json:"started_at,omitempty"`
	LastRunAt           time.Time `json:"last_run_at,omitempty"`
	NextRunAt           time.Time `json:"next_run_at,omitempty"`
	StoppedAt           time.Time `json:"stopped_at,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
	Events              []Event   `json:"events"`
}

type Service struct {
	mu      sync.Mutex
	store   *store.Store
	mailbox *mailboxservice.Service
	state   State
	cancel  context.CancelFunc
	nextID  int64
}

func NewService(state *store.Store, mailbox *mailboxservice.Service) *Service {
	return &Service{store: state, mailbox: mailbox, state: State{Status: "idle", Events: []Event{}}}
}

func (s *Service) Start(parent context.Context, cfg Config) (State, error) {
	cfg = s.normalizeConfig(cfg)
	if len(cfg.AccountIDs) == 0 {
		return State{}, errors.New("请至少选择一个 Apple 账号")
	}
	for _, accountID := range cfg.AccountIDs {
		if _, ok := s.store.FindAppleAccount(accountID); !ok {
			return State{}, fmt.Errorf("Apple 账号不存在：%s", accountID)
		}
		if _, ok := s.store.ICloudSessionByAccountID(accountID); !ok {
			return State{}, fmt.Errorf("Apple 账号没有可用登录态：%s", accountID)
		}
	}

	s.mu.Lock()
	if s.state.Running {
		s.mu.Unlock()
		return State{}, errors.New("定时创建已经在运行，请先停止")
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.state = State{
		Running:             true,
		Status:              "running",
		AccountIDs:          append([]string(nil), cfg.AccountIDs...),
		Label:               cfg.Label,
		Note:                cfg.Note,
		CreateChannel:       cfg.CreateChannel,
		IntervalSeconds:     s.interval(cfg),
		RoundIntervalSecond: cfg.RoundIntervalSecond,
		StartedAt:           time.Now(),
		Events:              []Event{},
	}
	s.addEventLocked(Event{Type: "started", Message: "定时创建已启动"})
	out := s.snapshotLocked()
	s.mu.Unlock()

	go s.run(ctx, cfg)
	return out, nil
}

func (s *Service) Stop(message string) State {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.state.Running {
		s.state.Running = false
		s.state.Status = "stopped"
		s.state.StoppedAt = time.Now()
		if strings.TrimSpace(message) == "" {
			message = "定时创建已停止"
		}
		s.addEventLocked(Event{Type: "stopped", Message: message})
	}
	return s.snapshotLocked()
}

func (s *Service) ClearEvents() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Events = []Event{}
	return s.snapshotLocked()
}

func (s *Service) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Service) RecordManualSuccess(accountID string, mailbox domain.Mailbox) State {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Success++
	s.state.LastRunAt = time.Now()
	s.state.LastError = ""
	s.addEventLocked(Event{Type: "created", AccountID: accountID, MailboxID: mailbox.ID, Email: mailbox.Email, Message: "单次创建隐私邮箱成功"})
	return s.snapshotLocked()
}

func (s *Service) RecordManualFailure(accountID string, createErr error) State {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Failed++
	s.state.LastRunAt = time.Now()
	s.state.LastError = createErr.Error()
	s.addEventLocked(Event{Type: "failed", AccountID: accountID, Message: "单次创建隐私邮箱失败", Error: createErr.Error()})
	return s.snapshotLocked()
}

func (s *Service) run(ctx context.Context, cfg Config) {
	interval := time.Duration(s.interval(cfg)) * time.Second
	for {
		if ctx.Err() != nil {
			s.finish()
			return
		}
		s.runRound(ctx, cfg)
		if ctx.Err() != nil {
			s.finish()
			return
		}
		s.mu.Lock()
		s.state.Status = "waiting"
		s.state.NextRunAt = time.Now().Add(interval)
		s.addEventLocked(Event{Type: "waiting", Message: fmt.Sprintf("等待 %s 后开始下一轮", interval)})
		s.mu.Unlock()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.finish()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) runRound(ctx context.Context, cfg Config) {
	s.mu.Lock()
	s.state.BatchIndex++
	batch := s.state.BatchIndex
	s.state.Status = "creating"
	s.state.LastRunAt = time.Now()
	s.state.NextRunAt = time.Time{}
	s.addEventLocked(Event{Type: "round_started", Message: fmt.Sprintf("开始第 %d 轮定时创建", batch)})
	s.mu.Unlock()

	for index, accountID := range cfg.AccountIDs {
		if ctx.Err() != nil {
			return
		}
		mailbox, err := s.mailbox.Create(ctx, accountID, cfg.Label, cfg.Note, cfg.CreateChannel)
		s.mu.Lock()
		if err != nil {
			s.state.Failed++
			s.state.LastError = err.Error()
			s.addEventLocked(Event{Type: "failed", AccountID: accountID, Message: "创建隐私邮箱失败", Error: err.Error()})
		} else {
			s.state.Success++
			s.state.LastError = ""
			s.addEventLocked(Event{Type: "created", AccountID: accountID, MailboxID: mailbox.ID, Email: mailbox.Email, Message: "已创建隐私邮箱 " + mailbox.Email})
		}
		s.mu.Unlock()
		if index < len(cfg.AccountIDs)-1 && cfg.RoundIntervalSecond > 0 {
			timer := time.NewTimer(time.Duration(cfg.RoundIntervalSecond) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func (s *Service) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.Running {
		return
	}
	s.state.Running = false
	s.state.Status = "stopped"
	s.state.StoppedAt = time.Now()
	s.cancel = nil
	s.addEventLocked(Event{Type: "stopped", Message: "定时创建已停止"})
}

func (s *Service) normalizeConfig(cfg Config) Config {
	seen := map[string]bool{}
	ids := make([]string, 0, len(cfg.AccountIDs))
	for _, id := range cfg.AccountIDs {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	cfg.AccountIDs = ids
	cfg.Label = strings.TrimSpace(cfg.Label)
	cfg.Note = strings.TrimSpace(cfg.Note)
	cfg.CreateChannel = strings.ToLower(strings.TrimSpace(cfg.CreateChannel))
	switch cfg.CreateChannel {
	case "apple_account", "icloud_web":
	default:
		cfg.CreateChannel = "auto"
	}
	if cfg.IntervalMinutes <= 0 && cfg.IntervalSeconds <= 0 {
		cfg.IntervalMinutes = 60
	}
	if cfg.RoundIntervalSecond <= 0 {
		cfg.RoundIntervalSecond = 5
	}
	return cfg
}

func (s *Service) interval(cfg Config) int {
	if cfg.IntervalSeconds > 0 {
		if cfg.IntervalSeconds < 1 {
			return 1
		}
		return cfg.IntervalSeconds
	}
	return cfg.IntervalMinutes * 60
}

func (s *Service) addEventLocked(event Event) {
	s.nextID++
	event.ID = s.nextID
	if event.At.IsZero() {
		event.At = time.Now()
	}
	s.state.Events = append(s.state.Events, event)
	if len(s.state.Events) > maxEvents {
		s.state.Events = append([]Event(nil), s.state.Events[len(s.state.Events)-maxEvents:]...)
	}
}

func (s *Service) snapshotLocked() State {
	out := s.state
	out.AccountIDs = append([]string(nil), s.state.AccountIDs...)
	out.Events = append([]Event(nil), s.state.Events...)
	return out
}

func DefaultConfig(state *store.Store) Config {
	settings := state.CreateSettings()
	accountIDs := append([]string(nil), settings.AccountIDs...)
	if len(accountIDs) == 0 {
		for _, account := range state.AppleAccounts() {
			if account.Status == domain.StatusActive {
				accountIDs = append(accountIDs, account.ID)
				break
			}
		}
	}
	return Config{
		AccountIDs:          accountIDs,
		Label:               settings.Label,
		Note:                settings.Note,
		CreateChannel:       settings.SchedulerCreateChannel,
		IntervalMinutes:     settings.SchedulerIntervalMinutes,
		RoundIntervalSecond: settings.SchedulerRoundIntervalSeconds,
	}
}
