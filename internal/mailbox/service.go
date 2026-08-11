package mailbox

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"icloud-privacy-mail-v2/internal/config"
	"icloud-privacy-mail-v2/internal/domain"
	"icloud-privacy-mail-v2/internal/protocol"
	"icloud-privacy-mail-v2/internal/store"
)

type Service struct {
	cfg          config.Config
	store        *store.Store
	client       *protocol.ICloudClient
	deleteClient remoteMailboxDeleteClient
	createMu     sync.Mutex
	syncMu       sync.Mutex
	syncs        map[string]*syncCall
}

type remoteMailboxDeleteClient interface {
	ListPrivacyMailboxes(context.Context, protocol.ICloudSession) ([]protocol.ICloudRemoteMailbox, error)
	DeletePrivacyMailbox(context.Context, protocol.ICloudSession, string) error
	MoveRemoteMessagesToTrash(context.Context, protocol.ICloudSession, []string) (protocol.ICloudMailCleanupResult, error)
	EmptyTrash(context.Context, protocol.ICloudSession) (int, error)
}

const mailboxCodeFreshWindow = 5 * time.Minute

type syncCall struct {
	done  chan struct{}
	count int
	err   error
}

type CodeResult struct {
	Email      string    `json:"email"`
	Code       string    `json:"code"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	ReceivedAt time.Time `json:"received_at"`
	MessageID  string    `json:"message_id"`
}

type CodeQuery struct {
	After         time.Time
	Keyword       string
	SkipMessageID string
	IncludeServed bool
	MarkAsServed  bool
}

type RemoteCleanupOptions struct {
	AccountID  string `json:"account_id,omitempty"`
	MoveSynced bool   `json:"move_synced"`
	EmptyTrash bool   `json:"empty_trash"`
}

type RemoteCleanupFailure struct {
	MailboxID string `json:"mailbox_id"`
	Email     string `json:"email"`
	Error     string `json:"error"`
}

type RemoteCleanupBatchResult struct {
	Cleanup         protocol.ICloudMailCleanupResult `json:"cleanup"`
	Mailboxes       int                              `json:"mailboxes"`
	FailedMailboxes int                              `json:"failed_mailboxes"`
	Failures        []RemoteCleanupFailure           `json:"failures,omitempty"`
}

func NewService(cfg config.Config, state *store.Store) *Service {
	client := protocol.NewICloudClient()
	return &Service{cfg: cfg, store: state, client: client, deleteClient: client, syncs: make(map[string]*syncCall)}
}

// ImportLocal 把已有隐私邮箱绑定到 Apple 账号，不调用 Apple 创建接口。
func (s *Service) ImportLocal(accountID, email, label, note string) (domain.Mailbox, bool, error) {
	accountID = strings.TrimSpace(accountID)
	if _, ok := s.store.FindAppleAccount(accountID); !ok {
		return domain.Mailbox{}, false, errors.New("Apple 账号不存在")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(strings.TrimSpace(address.Address), email) {
		return domain.Mailbox{}, false, errors.New("邮箱地址格式不正确")
	}
	mailbox, created, err := s.store.UpsertMailboxFromRemote(accountID, domain.RemoteMailbox{
		Email: email, Label: strings.TrimSpace(label), IsActive: true, Origin: "manual",
	}, firstNonEmpty(note, "手动导入本地邮箱"))
	return mailbox, created, err
}

func (s *Service) Create(ctx context.Context, accountID, label, note, channel string) (domain.Mailbox, error) {
	session, ok := s.store.ICloudSessionByAccountID(accountID)
	if !ok {
		return domain.Mailbox{}, errors.New("Apple 账号登录态不存在")
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()
	label = s.store.NextMailboxLabel(label)
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "" {
		channel = "auto"
	}
	var remote protocol.ICloudRemoteMailbox
	var err error
	if channel == "auto" || channel == "apple_account" {
		if _, saved := protocol.LoginStateForKind(session, domain.LoginStateAppleAccount); saved {
			var updated domain.ICloudSession
			remote, updated, err = s.client.CreatePrivacyMailboxWithAppleAccount(ctx, session, s.cfg.AppleAccountAPIKey, label, note)
			if err == nil {
				_, _ = s.store.SaveICloudSession(updated)
			}
		} else if channel == "apple_account" {
			err = errors.New("该账号没有 Apple Account 新接口登录态")
		}
	}
	if remote.Email == "" && (channel == "auto" || channel == "icloud_web") {
		remote, err = s.client.CreatePrivacyMailbox(ctx, session, label, note)
	}
	if err != nil {
		return domain.Mailbox{}, err
	}
	mailbox, _, err := s.store.UpsertMailboxFromRemote(accountID, remoteMailbox(remote), note)
	return mailbox, err
}

func (s *Service) SyncRemote(ctx context.Context, accountID string) ([]domain.Mailbox, error) {
	session, ok := s.store.ICloudSessionByAccountID(accountID)
	if !ok {
		return nil, errors.New("Apple 账号登录态不存在")
	}
	remotes, err := s.client.ListPrivacyMailboxes(ctx, session)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Mailbox, 0, len(remotes))
	for _, remote := range remotes {
		mailbox, _, saveErr := s.store.UpsertMailboxFromRemote(accountID, remoteMailbox(remote), "从 Apple 服务器同步")
		if saveErr != nil {
			return out, saveErr
		}
		out = append(out, mailbox)
	}
	return out, nil
}

func (s *Service) DeleteRemote(ctx context.Context, mailboxID string) error {
	mailbox, ok := s.store.FindMailboxByID(mailboxID)
	if !ok {
		return errors.New("邮箱不存在")
	}
	session, ok := s.store.ICloudSessionByAccountID(mailbox.AccountID)
	if !ok {
		return errors.New("对应 Apple 账号登录态不存在")
	}
	if err := s.cleanupMailboxMessagesBeforeDelete(ctx, mailboxID, session); err != nil {
		return err
	}
	client := s.deleteClient
	if client == nil {
		client = s.client
	}
	remotes, err := client.ListPrivacyMailboxes(ctx, session)
	if err != nil {
		return err
	}
	remote, exists := matchRemote(remotes, mailbox.AnonymousID, mailbox.Email)
	if exists {
		if err := client.DeletePrivacyMailbox(ctx, session, remote.AnonymousID); err != nil {
			return err
		}
	}
	confirmed, err := client.ListPrivacyMailboxes(ctx, session)
	if err != nil {
		return fmt.Errorf("Apple 删除后确认失败：%w", err)
	}
	if _, stillExists := matchRemote(confirmed, firstNonEmpty(remote.AnonymousID, mailbox.AnonymousID), mailbox.Email); stillExists {
		return errors.New("Apple 服务器仍然存在该隐私邮箱，本地记录已保留")
	}
	return s.store.DeleteMailbox(mailboxID)
}

func (s *Service) DeleteLocal(mailboxID string) error {
	if _, err := s.store.DeleteMailboxMessages(mailboxID); err != nil {
		return fmt.Errorf("删除邮箱前清理本地邮件失败：%w", err)
	}
	return s.store.DeleteMailbox(mailboxID)
}

func (s *Service) cleanupMailboxMessagesBeforeDelete(ctx context.Context, mailboxID string, session protocol.ICloudSession) error {
	client := s.deleteClient
	if client == nil {
		client = s.client
	}
	remoteIDs := remoteMessageIDs(s.store.MessagesForMailbox(mailboxID))
	if _, err := client.MoveRemoteMessagesToTrash(ctx, session, remoteIDs); err != nil {
		return fmt.Errorf("删除邮箱前清理 Apple 远端邮件失败：%w", err)
	}
	if _, err := client.EmptyTrash(ctx, session); err != nil {
		return fmt.Errorf("删除邮箱前清空 Apple 废纸篓失败：%w", err)
	}
	if _, err := s.store.DeleteMailboxMessages(mailboxID); err != nil {
		return fmt.Errorf("删除邮箱前清理本地邮件失败：%w", err)
	}
	return nil
}

func (s *Service) CleanRemoteMessages(ctx context.Context, mailboxID string, options RemoteCleanupOptions) (protocol.ICloudMailCleanupResult, error) {
	mailbox, ok := s.store.FindMailboxByID(mailboxID)
	if !ok {
		return protocol.ICloudMailCleanupResult{}, errors.New("邮箱不存在")
	}
	session, ok := s.store.ICloudSessionByAccountID(mailbox.AccountID)
	if !ok {
		return protocol.ICloudMailCleanupResult{}, errors.New("对应 Apple 账号登录态不存在")
	}
	options = normalizeRemoteCleanupOptions(options)
	result := protocol.ICloudMailCleanupResult{}
	if options.MoveSynced {
		moved, err := s.client.MoveRemoteMessagesToTrash(ctx, session, remoteMessageIDs(s.store.MessagesForMailbox(mailboxID)))
		result.MovedToTrash += moved.MovedToTrash
		result.Skipped += moved.Skipped
		if err != nil {
			return result, err
		}
		localIDs := append(append([]string(nil), moved.MovedRemoteIDs...), moved.AbsentRemoteIDs...)
		removed, err := s.store.DeleteMailboxMessagesByRemoteIDs(mailboxID, localIDs)
		if err != nil {
			return result, err
		}
		result.LocalRemoved += removed
	}
	if options.EmptyTrash {
		destroyed, err := s.client.EmptyTrash(ctx, session)
		result.Destroyed += destroyed
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Service) CleanRemoteMailboxes(ctx context.Context, options RemoteCleanupOptions) RemoteCleanupBatchResult {
	options = normalizeRemoteCleanupOptions(options)
	options.AccountID = strings.TrimSpace(options.AccountID)
	result := RemoteCleanupBatchResult{}
	cleanedTrash := make(map[string]bool)
	for _, mailbox := range s.store.AllMailboxes() {
		if ctx.Err() != nil {
			break
		}
		if options.AccountID != "" && mailbox.AccountID != options.AccountID {
			continue
		}
		if !mailbox.ICloudActive || mailbox.Status == domain.StatusDisabled {
			result.Cleanup.Skipped++
			continue
		}
		session, ok := s.store.ICloudSessionByAccountID(mailbox.AccountID)
		if !ok {
			result.Cleanup.Skipped++
			continue
		}
		if options.MoveSynced {
			moved, err := s.client.MoveRemoteMessagesToTrash(ctx, session, remoteMessageIDs(s.store.MessagesForMailbox(mailbox.ID)))
			result.Cleanup.MovedToTrash += moved.MovedToTrash
			result.Cleanup.Skipped += moved.Skipped
			if err != nil {
				result.FailedMailboxes++
				result.Failures = append(result.Failures, RemoteCleanupFailure{MailboxID: mailbox.ID, Email: mailbox.Email, Error: err.Error()})
				continue
			}
			localIDs := append(append([]string(nil), moved.MovedRemoteIDs...), moved.AbsentRemoteIDs...)
			removed, err := s.store.DeleteMailboxMessagesByRemoteIDs(mailbox.ID, localIDs)
			if err != nil {
				result.FailedMailboxes++
				result.Failures = append(result.Failures, RemoteCleanupFailure{MailboxID: mailbox.ID, Email: mailbox.Email, Error: err.Error()})
				continue
			}
			result.Cleanup.LocalRemoved += removed
		}
		result.Mailboxes++
		sessionKey := firstNonEmpty(session.AccountID, session.DSID, session.AppleID, mailbox.AccountID)
		if options.EmptyTrash && !cleanedTrash[sessionKey] {
			destroyed, err := s.client.EmptyTrash(ctx, session)
			result.Cleanup.Destroyed += destroyed
			if err != nil {
				result.FailedMailboxes++
				result.Failures = append(result.Failures, RemoteCleanupFailure{MailboxID: mailbox.ID, Email: mailbox.Email, Error: err.Error()})
				continue
			}
			cleanedTrash[sessionKey] = true
		}
	}
	return result
}

func (s *Service) SyncMessages(ctx context.Context, mailboxID string) (int, error) {
	mailbox, ok := s.store.FindMailboxByID(mailboxID)
	if !ok {
		return 0, errors.New("邮箱不存在")
	}
	mailboxes := s.mailboxesForAccount(mailbox.AccountID)
	found := false
	for _, item := range mailboxes {
		if item.ID == mailbox.ID {
			found = true
			break
		}
	}
	if !found {
		mailboxes = append(mailboxes, mailbox)
	}
	return s.SyncMailboxBatch(ctx, mailboxes, time.Time{}, "", s.cfg.MailWatcherFetchLimit)
}

// SyncMailboxBatch 按 Apple 账号批量拉取一次收件箱，再把邮件分发给对应隐私邮箱。
func (s *Service) SyncMailboxBatch(ctx context.Context, mailboxes []domain.Mailbox, after time.Time, keyword string, maxMessages int) (int, error) {
	if len(mailboxes) == 0 {
		return 0, nil
	}
	if maxMessages <= 0 {
		maxMessages = s.cfg.MailWatcherFetchLimit
	}
	groups := make(map[string][]domain.Mailbox)
	order := make([]string, 0)
	for _, mailbox := range mailboxes {
		key := firstNonEmpty(mailbox.AccountID, mailbox.OwnerID, "__legacy__")
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], mailbox)
	}
	total := 0
	for _, key := range order {
		count, err := s.syncGroup(ctx, key, groups[key], after, keyword, maxMessages)
		total += count
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (s *Service) syncGroup(ctx context.Context, key string, mailboxes []domain.Mailbox, after time.Time, keyword string, maxMessages int) (int, error) {
	s.syncMu.Lock()
	if running := s.syncs[key]; running != nil {
		s.syncMu.Unlock()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-running.done:
			return running.count, running.err
		}
	}
	call := &syncCall{done: make(chan struct{})}
	s.syncs[key] = call
	s.syncMu.Unlock()

	call.count, call.err = s.syncGroupNow(ctx, mailboxes, after, keyword, maxMessages)
	s.syncMu.Lock()
	delete(s.syncs, key)
	close(call.done)
	s.syncMu.Unlock()
	return call.count, call.err
}

func (s *Service) syncGroupNow(ctx context.Context, mailboxes []domain.Mailbox, after time.Time, keyword string, maxMessages int) (int, error) {
	refreshed := make([]domain.Mailbox, 0, len(mailboxes))
	for _, mailbox := range mailboxes {
		if current, ok := s.store.FindMailboxByID(mailbox.ID); ok {
			refreshed = append(refreshed, current)
		}
	}
	if len(refreshed) == 0 {
		return 0, nil
	}
	session, ok := s.store.ICloudSessionByAccountID(refreshed[0].AccountID)
	if !ok {
		return 0, errors.New("对应 Apple 账号登录态不存在")
	}

	messagesByMailbox := make(map[string][]protocol.ICloudSyncedMessage)
	lastUID := ""
	source := "icloud"
	if imapState, saved := protocol.LoginStateForKind(session, domain.LoginStateICloudIMAP); saved {
		result, err := protocol.SyncICloudIMAPMessagesDetailed(ctx, imapState, refreshed, after, keyword, maxMessages)
		if err != nil {
			return 0, err
		}
		messagesByMailbox = result.MessagesByMailbox
		lastUID = strings.TrimSpace(result.LastUID)
		source = "imap"
		if lastUID != "" {
			imapState.IMAPLastSyncAt = time.Now()
			imapState.IMAPLastSyncUID = lastUID
			session = protocol.WithLoginState(session, imapState)
			if _, err := s.store.SaveICloudSession(session); err != nil {
				return 0, err
			}
		}
	} else {
		var err error
		messagesByMailbox, err = s.client.SyncMailboxMessagesBatch(ctx, session, refreshed, after, keyword, maxMessages)
		if err != nil {
			return 0, err
		}
	}

	created := 0
	syncedAt := time.Now()
	for _, mailbox := range refreshed {
		mailboxUID := firstNonEmpty(lastUID, mailbox.LastSyncUID)
		for _, message := range messagesByMailbox[mailbox.ID] {
			remoteID := firstNonEmpty(message.RemoteID, message.UID)
			_, added, err := s.store.UpsertMessage(mailbox.ID, remoteID, source, message.Subject, message.From, message.Body, message.ReceivedAt)
			if err != nil {
				return created, err
			}
			if added {
				created++
			}
		}
		if _, err := s.store.SetMailboxSyncCursor(mailbox.ID, syncedAt, mailboxUID); err != nil {
			return created, err
		}
	}
	return created, nil
}

func (s *Service) Code(ctx context.Context, mailboxID string, after time.Time, keyword string, allowStale bool) (CodeResult, error) {
	mailbox, ok := s.store.FindMailboxByID(mailboxID)
	if !ok {
		return CodeResult{}, errors.New("邮箱不存在")
	}
	query := CodeQuery{After: after, Keyword: keyword, SkipMessageID: mailbox.LastCodeMessageID, MarkAsServed: true}
	if result, found, err := s.findCode(mailbox, query); found || err != nil {
		return result, err
	}
	if _, err := s.SyncMessages(ctx, mailboxID); err != nil && !allowStale {
		return CodeResult{}, err
	}
	mailbox, _ = s.store.FindMailboxByID(mailboxID)
	if result, found, err := s.findCode(mailbox, query); found || err != nil {
		return result, err
	}
	return CodeResult{}, errors.New("暂未收到验证码")
}

// CachedCode 只读取本地邮件缓存，不触发 Apple 或 IMAP 网络请求。
func (s *Service) CachedCode(mailboxID string, after time.Time, keyword string, allowStale bool) (CodeResult, bool, error) {
	mailbox, ok := s.store.FindMailboxByID(mailboxID)
	if !ok {
		return CodeResult{}, false, errors.New("邮箱不存在")
	}
	query := CodeQuery{After: after, Keyword: keyword, SkipMessageID: mailbox.LastCodeMessageID, MarkAsServed: true}
	return s.findCode(mailbox, query)
}

// CachedCodeWithQuery 支持预览、缓存读取和固定请求起点，便于合并并发取码请求。
func (s *Service) CachedCodeWithQuery(mailboxID string, query CodeQuery) (CodeResult, bool, error) {
	mailbox, ok := s.store.FindMailboxByID(mailboxID)
	if !ok {
		return CodeResult{}, false, errors.New("邮箱不存在")
	}
	return s.findCode(mailbox, query)
}

func (s *Service) findCode(mailbox domain.Mailbox, query CodeQuery) (CodeResult, bool, error) {
	keyword := strings.TrimSpace(query.Keyword)
	if keyword == "" {
		keyword = "OpenAI"
	}
	after := codeAfter(query.After, time.Now())
	skipMessageID := strings.TrimSpace(query.SkipMessageID)
	if query.IncludeServed {
		skipMessageID = ""
	}
	for _, message := range s.store.MessagesForMailbox(mailbox.ID) {
		messageTime := message.ReceivedAt
		if messageTime.IsZero() {
			messageTime = message.CreatedAt
		}
		if messageTime.IsZero() || messageTime.Before(after) {
			continue
		}
		text := message.Subject + " " + message.From + " " + message.Body
		if !strings.EqualFold(keyword, "OpenAI") && !strings.Contains(strings.ToLower(text), strings.ToLower(keyword)) {
			continue
		}
		if skipMessageID != "" && message.ID == skipMessageID {
			continue
		}
		code := protocol.ExtractOTP(message.Subject + "\n" + message.Body)
		if code == "" {
			continue
		}
		if query.MarkAsServed {
			if err := s.store.SetMailboxLastCode(mailbox.ID, message.ID, time.Now()); err != nil {
				return CodeResult{}, false, err
			}
		}
		return CodeResult{Email: mailbox.Email, Code: code, Subject: message.Subject, From: message.From, ReceivedAt: messageTime, MessageID: message.ID}, true, nil
	}
	return CodeResult{}, false, nil
}

func codeAfter(after, now time.Time) time.Time {
	cutoff := now.Add(-mailboxCodeFreshWindow)
	if after.After(cutoff) {
		return after
	}
	return cutoff
}

func (s *Service) mailboxesForAccount(accountID string) []domain.Mailbox {
	accountID = strings.TrimSpace(accountID)
	out := make([]domain.Mailbox, 0)
	for _, mailbox := range s.store.AllMailboxes() {
		if mailbox.AccountID != accountID || !mailbox.ICloudActive || mailbox.Status == domain.StatusDisabled {
			continue
		}
		out = append(out, mailbox)
	}
	return out
}

func normalizeRemoteCleanupOptions(options RemoteCleanupOptions) RemoteCleanupOptions {
	if !options.MoveSynced && !options.EmptyTrash {
		options.MoveSynced = true
		options.EmptyTrash = true
	}
	return options
}

func remoteMessageIDs(messages []domain.Message) []string {
	out := make([]string, 0, len(messages))
	seen := make(map[string]bool)
	for _, message := range messages {
		remoteID := strings.TrimSpace(message.RemoteID)
		if remoteID == "" || seen[remoteID] {
			continue
		}
		seen[remoteID] = true
		out = append(out, remoteID)
	}
	return out
}

func remoteMailbox(remote protocol.ICloudRemoteMailbox) domain.RemoteMailbox {
	return domain.RemoteMailbox{AnonymousID: remote.AnonymousID, Email: remote.Email, Label: remote.Label, Note: remote.Note, IsActive: remote.IsActive, Origin: remote.Origin}
}

func matchRemote(remotes []protocol.ICloudRemoteMailbox, anonymousID, email string) (protocol.ICloudRemoteMailbox, bool) {
	for _, remote := range remotes {
		if strings.TrimSpace(anonymousID) != "" && remote.AnonymousID == strings.TrimSpace(anonymousID) {
			return remote, true
		}
		if strings.EqualFold(remote.Email, strings.TrimSpace(email)) {
			return remote, true
		}
	}
	return protocol.ICloudRemoteMailbox{}, false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
