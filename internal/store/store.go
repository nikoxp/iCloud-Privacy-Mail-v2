package store

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"icloud-privacy-mail-v2/internal/domain"
)

type Store struct {
	mu    sync.RWMutex
	path  string
	state domain.State
}

func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = filepath.Join("data", "state.json")
	}
	now := time.Now()
	s := &Store{
		path: path,
		state: domain.State{
			SchemaVersion:  domain.SchemaVersion,
			NextID:         1,
			Settings:       domain.DefaultSettings(),
			CreateSettings: domain.DefaultCreateSettings(),
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.saveLocked()
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return s.saveLocked()
	}
	if err := json.Unmarshal(data, &s.state); err != nil {
		return fmt.Errorf("读取状态文件失败：%w", err)
	}
	s.normalizeLocked()
	return nil
}

func (s *Store) normalizeLocked() {
	s.state.SchemaVersion = domain.SchemaVersion
	s.migrateLegacyICloudSessionsLocked()
	if s.state.NextID <= 0 {
		s.state.NextID = 1
	}
	s.ensureNextIDLocked()
	if s.state.Settings.MailboxPageSize <= 0 {
		s.state.Settings = domain.DefaultSettings()
	}
	// Apple 协议层已经完成迁移，旧状态文件载入后也应开放账号模块。
	s.state.Settings.AppleAccountModuleReady = true
	normalizeCreateSettings(&s.state.CreateSettings)
	if s.state.CreatedAt.IsZero() {
		s.state.CreatedAt = time.Now()
	}
	if s.state.UpdatedAt.IsZero() {
		s.state.UpdatedAt = s.state.CreatedAt
	}
}

func (s *Store) migrateLegacyICloudSessionsLocked() {
	if len(s.state.ICloudSessions) > 0 {
		return
	}
	rawStates := append([]json.RawMessage(nil), s.state.LegacyICloudStates...)
	if len(s.state.LegacyICloudSession) > 0 && string(s.state.LegacyICloudSession) != "null" {
		rawStates = append(rawStates, s.state.LegacyICloudSession)
	}
	for _, raw := range rawStates {
		var session domain.ICloudSession
		if err := json.Unmarshal(raw, &session); err == nil {
			s.state.ICloudSessions = append(s.state.ICloudSessions, session)
		}
	}
	if len(s.state.ICloudSessions) > 0 {
		s.state.LegacyICloudSession = nil
		s.state.LegacyICloudStates = nil
	}
}

func (s *Store) ensureNextIDLocked() {
	maxID := s.state.NextID - 1
	consider := func(value string) {
		index := strings.LastIndex(value, "_")
		if index < 0 || index == len(value)-1 {
			return
		}
		parsed, err := strconv.Atoi(value[index+1:])
		if err == nil && parsed > maxID {
			maxID = parsed
		}
	}
	if s.state.Admin != nil {
		consider(s.state.Admin.ID)
	}
	for _, account := range s.state.AppleAccounts {
		consider(account.ID)
	}
	for _, mailbox := range s.state.Mailboxes {
		consider(mailbox.ID)
	}
	for _, message := range s.state.Messages {
		consider(message.ID)
	}
	for _, event := range s.state.Events {
		consider(event.ID)
	}
	if s.state.NextID <= maxID {
		s.state.NextID = maxID + 1
	}
}

func normalizeCreateSettings(settings *domain.CreateSettings) {
	defaults := domain.DefaultCreateSettings()
	if strings.TrimSpace(settings.Label) == "" {
		settings.Label = defaults.Label
	}
	if strings.TrimSpace(settings.CreateChannel) == "" {
		settings.CreateChannel = defaults.CreateChannel
	}
	if strings.TrimSpace(settings.SchedulerCreateChannel) == "" {
		settings.SchedulerCreateChannel = defaults.SchedulerCreateChannel
	}
	if strings.TrimSpace(settings.AppleAccountTwoFactorMethod) == "" {
		settings.AppleAccountTwoFactorMethod = defaults.AppleAccountTwoFactorMethod
	}
	if strings.TrimSpace(settings.ICloudWebTwoFactorMethod) == "" {
		settings.ICloudWebTwoFactorMethod = defaults.ICloudWebTwoFactorMethod
	}
	if settings.SchedulerIntervalMinutes <= 0 {
		settings.SchedulerIntervalMinutes = defaults.SchedulerIntervalMinutes
	}
	if settings.SchedulerRoundIntervalSeconds <= 0 {
		settings.SchedulerRoundIntervalSeconds = defaults.SchedulerRoundIntervalSeconds
	}
	if settings.MailboxPageSize <= 0 {
		settings.MailboxPageSize = defaults.MailboxPageSize
	}
}

// NextMailboxLabel 根据本地已有邮箱标签生成下一个连续编号。
func (s *Store) NextMailboxLabel(prefix string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = domain.DefaultCreateSettings().Label
	}
	marker := prefix + "_"
	maxNumber := 0
	for _, mailbox := range s.state.Mailboxes {
		label := strings.TrimSpace(mailbox.Label)
		if len(label) <= len(marker) || !strings.EqualFold(label[:len(marker)], marker) {
			continue
		}
		number, err := strconv.Atoi(label[len(marker):])
		if err == nil && number > maxNumber {
			maxNumber = number
		}
	}
	return fmt.Sprintf("%s_%d", prefix, maxNumber+1)
}

func (s *Store) saveLocked() error {
	s.state.SchemaVersion = domain.SchemaVersion
	s.state.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func (s *Store) Snapshot() domain.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, _ := json.Marshal(s.state)
	var out domain.State
	_ = json.Unmarshal(data, &out)
	return out
}

func (s *Store) SetupAdmin(username, passwordHash string) (domain.Admin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Admin != nil {
		return domain.Admin{}, errors.New("管理员已经初始化")
	}
	now := time.Now()
	admin := domain.Admin{
		ID:           s.nextIDLocked("admin"),
		Username:     strings.ToLower(strings.TrimSpace(username)),
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.state.Admin = &admin
	s.appendEventLocked("info", "auth", "已完成单管理员初始化")
	return admin, s.saveLocked()
}

func (s *Store) Admin() (domain.Admin, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Admin == nil {
		return domain.Admin{}, false
	}
	return *s.state.Admin, true
}

func (s *Store) MarkLogin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Admin == nil {
		return errors.New("管理员尚未初始化")
	}
	now := time.Now()
	s.state.Admin.LastLoginAt = now
	s.state.Admin.UpdatedAt = now
	s.appendEventLocked("info", "auth", "管理员已登录")
	return s.saveLocked()
}

func (s *Store) SaveSession(tokenHash string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	filtered := s.state.Sessions[:0]
	for _, session := range s.state.Sessions {
		if session.ExpiresAt.After(now) {
			filtered = append(filtered, session)
		}
	}
	s.state.Sessions = append(filtered, domain.WebSession{
		TokenHash:  tokenHash,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  expiresAt,
	})
	return s.saveLocked()
}

func (s *Store) ValidateSession(tokenHash string) (domain.Admin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Admin == nil || strings.TrimSpace(tokenHash) == "" {
		return domain.Admin{}, false
	}
	now := time.Now()
	for i := range s.state.Sessions {
		session := &s.state.Sessions[i]
		if session.ExpiresAt.Before(now) {
			continue
		}
		if constantTimeEqual(session.TokenHash, tokenHash) {
			session.LastSeenAt = now
			return *s.state.Admin, true
		}
	}
	return domain.Admin{}, false
}

func (s *Store) DeleteSession(tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.state.Sessions[:0]
	for _, session := range s.state.Sessions {
		if !constantTimeEqual(session.TokenHash, tokenHash) {
			filtered = append(filtered, session)
		}
	}
	s.state.Sessions = filtered
	return s.saveLocked()
}

func (s *Store) Dashboard() domain.Dashboard {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := domain.Dashboard{
		AppleAccountCount: len(s.state.AppleAccounts),
		MailboxCount:      len(s.state.Mailboxes),
		MessageCount:      len(s.state.Messages),
	}
	for _, account := range s.state.AppleAccounts {
		if strings.EqualFold(account.Status, "active") || strings.EqualFold(account.ICloudStatus, "active") {
			out.ActiveAccountCount++
		}
	}
	for _, mailbox := range s.state.Mailboxes {
		if mailbox.APIActive && mailbox.ICloudActive && strings.EqualFold(mailbox.Status, "available") {
			out.AvailableCount++
		}
	}
	start := 0
	if len(s.state.Events) > 30 {
		start = len(s.state.Events) - 30
	}
	out.Events = append([]domain.Event(nil), s.state.Events[start:]...)
	sort.Slice(out.Events, func(i, j int) bool { return out.Events[i].CreatedAt.After(out.Events[j].CreatedAt) })
	return out
}

func (s *Store) AppleAccounts() []domain.AppleAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]domain.AppleAccount(nil), s.state.AppleAccounts...)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Store) Mailboxes(query, status, accountID string, page, pageSize int) domain.MailboxPage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	status = strings.ToLower(strings.TrimSpace(status))
	accountID = strings.TrimSpace(accountID)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = s.state.Settings.MailboxPageSize
	}
	if pageSize <= 0 {
		pageSize = domain.DefaultSettings().MailboxPageSize
	}
	if pageSize > 200 {
		pageSize = 200
	}
	items := make([]domain.Mailbox, 0, len(s.state.Mailboxes))
	for _, mailbox := range s.state.Mailboxes {
		if accountID != "" && mailbox.AccountID != accountID {
			continue
		}
		if status != "" && !strings.EqualFold(mailbox.Status, status) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(mailbox.Email+" "+mailbox.Label+" "+mailbox.Note), query) {
			continue
		}
		mailbox.APIToken = ""
		items = append(items, mailbox)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	total := len(items)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	return domain.MailboxPage{
		Items:      append([]domain.Mailbox(nil), items[start:end]...),
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

func (s *Store) Settings() domain.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Settings
}

func (s *Store) CreateSettings() domain.CreateSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.state.CreateSettings
	out.AccountIDs = append([]string(nil), out.AccountIDs...)
	return out
}

func (s *Store) SaveCreateSettings(settings domain.CreateSettings) (domain.CreateSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalizeCreateSettings(&settings)
	settings.UpdatedAt = time.Now()
	s.state.CreateSettings = settings
	s.appendEventLocked("info", "settings", "已保存邮箱创建设置")
	return settings, s.saveLocked()
}

func (s *Store) ICloudSessions() []domain.ICloudSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, _ := json.Marshal(s.state.ICloudSessions)
	var out []domain.ICloudSession
	_ = json.Unmarshal(data, &out)
	return out
}

func (s *Store) SaveSettings(settings domain.Settings) (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings.PublicAPIKey = strings.TrimSpace(settings.PublicAPIKey)
	if settings.MailboxPageSize <= 0 {
		settings.MailboxPageSize = domain.DefaultSettings().MailboxPageSize
	}
	if settings.MailboxPageSize > 200 {
		settings.MailboxPageSize = 200
	}
	// Apple 模块由后端能力决定，不允许通过页面关闭。
	settings.AppleAccountModuleReady = true
	s.state.Settings = settings
	s.appendEventLocked("info", "settings", "已保存系统设置")
	return settings, s.saveLocked()
}

func (s *Store) ReplaceState(state domain.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.normalizeLocked()
	s.state.Sessions = nil
	s.appendEventLocked("info", "migration", "已从旧项目导入数据")
	return s.saveLocked()
}

func (s *Store) ClearEvents() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Events = nil
	return s.saveLocked()
}

func (s *Store) RecordEvent(level, category, message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	level = strings.TrimSpace(level)
	if level == "" {
		level = "info"
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = "system"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendEventLocked(level, category, message)
	return s.saveLocked()
}

func (s *Store) nextIDLocked(prefix string) string {
	id := fmt.Sprintf("%s_%06d", prefix, s.state.NextID)
	s.state.NextID++
	return id
}

func (s *Store) appendEventLocked(level, category, message string) {
	s.state.Events = append(s.state.Events, domain.Event{
		ID:        s.nextIDLocked("evt"),
		Level:     level,
		Category:  category,
		Message:   message,
		CreatedAt: time.Now(),
	})
	if len(s.state.Events) > 500 {
		s.state.Events = append([]domain.Event(nil), s.state.Events[len(s.state.Events)-500:]...)
	}
}

func constantTimeEqual(candidate, expected string) bool {
	if candidate == "" || expected == "" || len(candidate) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(expected)) == 1
}
