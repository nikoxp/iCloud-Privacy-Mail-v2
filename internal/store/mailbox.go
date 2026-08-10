package store

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"icloud-privacy-mail-v2/internal/domain"
)

func (s *Store) AllMailboxes() []domain.Mailbox {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]domain.Mailbox(nil), s.state.Mailboxes...)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Store) FindMailboxByID(id string) (domain.Mailbox, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, mailbox := range s.state.Mailboxes {
		if mailbox.ID == strings.TrimSpace(id) {
			return mailbox, true
		}
	}
	return domain.Mailbox{}, false
}

func (s *Store) FindMailboxByEmail(email string) (domain.Mailbox, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	email = strings.ToLower(strings.TrimSpace(email))
	for _, mailbox := range s.state.Mailboxes {
		if strings.EqualFold(mailbox.Email, email) {
			return mailbox, true
		}
	}
	return domain.Mailbox{}, false
}

func (s *Store) UpsertMailboxFromRemote(accountID string, remote domain.RemoteMailbox, defaultNote string) (domain.Mailbox, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	email := strings.ToLower(strings.TrimSpace(remote.Email))
	if email == "" {
		return domain.Mailbox{}, false, errors.New("Apple 返回的隐私邮箱地址为空")
	}
	ownerID := ""
	if s.state.Admin != nil {
		ownerID = s.state.Admin.ID
	}
	now := time.Now()
	for i := range s.state.Mailboxes {
		if !strings.EqualFold(s.state.Mailboxes[i].Email, email) {
			continue
		}
		mailbox := &s.state.Mailboxes[i]
		mailbox.OwnerID = firstNonEmpty(mailbox.OwnerID, ownerID)
		mailbox.AccountID = firstNonEmpty(accountID, mailbox.AccountID)
		mailbox.AnonymousID = firstNonEmpty(remote.AnonymousID, mailbox.AnonymousID)
		mailbox.RemoteOrigin = firstNonEmpty(remote.Origin, mailbox.RemoteOrigin)
		if strings.TrimSpace(remote.Label) != "" {
			mailbox.Label = strings.TrimSpace(remote.Label)
		}
		mailbox.ICloudActive = remote.IsActive
		if strings.TrimSpace(mailbox.Note) == "" {
			mailbox.Note = firstNonEmpty(remote.Note, defaultNote)
		}
		mailbox.UpdatedAt = now
		return *mailbox, false, s.saveLocked()
	}
	token, err := randomAPIToken(24)
	if err != nil {
		return domain.Mailbox{}, false, err
	}
	status := domain.StatusAvailable
	if !remote.IsActive {
		status = domain.StatusDisabled
	}
	mailbox := domain.Mailbox{
		ID:           s.nextIDLocked("mbx"),
		OwnerID:      ownerID,
		AccountID:    strings.TrimSpace(accountID),
		AnonymousID:  strings.TrimSpace(remote.AnonymousID),
		RemoteOrigin: strings.TrimSpace(remote.Origin),
		Label:        firstNonEmpty(remote.Label, "隐私邮箱 "+now.Format("0102-150405")),
		Email:        email,
		APIToken:     token,
		APIActive:    true,
		ICloudActive: remote.IsActive,
		Status:       status,
		Note:         firstNonEmpty(remote.Note, defaultNote),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.state.Mailboxes = append(s.state.Mailboxes, mailbox)
	s.appendEventLocked("info", "mailbox", "已导入隐私邮箱 "+mailbox.Email)
	return mailbox, true, s.saveLocked()
}

func (s *Store) SetMailboxRemoteIdentity(id, anonymousID, origin string) (domain.Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.mailboxIndexLocked(id)
	if index < 0 {
		return domain.Mailbox{}, errors.New("邮箱不存在")
	}
	s.state.Mailboxes[index].AnonymousID = strings.TrimSpace(anonymousID)
	s.state.Mailboxes[index].RemoteOrigin = strings.TrimSpace(origin)
	s.state.Mailboxes[index].UpdatedAt = time.Now()
	return s.state.Mailboxes[index], s.saveLocked()
}

func (s *Store) SetMailboxStatus(id string, apiActive, icloudActive *bool, status string, note *string) (domain.Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.mailboxIndexLocked(id)
	if index < 0 {
		return domain.Mailbox{}, errors.New("邮箱不存在")
	}
	mailbox := &s.state.Mailboxes[index]
	if apiActive != nil {
		mailbox.APIActive = *apiActive
	}
	if icloudActive != nil {
		mailbox.ICloudActive = *icloudActive
	}
	if strings.TrimSpace(status) != "" {
		mailbox.Status = strings.TrimSpace(status)
	}
	if note != nil {
		mailbox.Note = strings.TrimSpace(*note)
	}
	mailbox.UpdatedAt = time.Now()
	s.appendEventLocked("info", "mailbox", "已更新邮箱状态 "+mailbox.Email)
	return *mailbox, s.saveLocked()
}

func (s *Store) DeleteMailbox(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.mailboxIndexLocked(id)
	if index < 0 {
		return errors.New("邮箱不存在")
	}
	email := s.state.Mailboxes[index].Email
	s.state.Mailboxes = append(s.state.Mailboxes[:index], s.state.Mailboxes[index+1:]...)
	messages := s.state.Messages[:0]
	for _, message := range s.state.Messages {
		if message.MailboxID != id {
			messages = append(messages, message)
		}
	}
	s.state.Messages = messages
	s.appendEventLocked("warning", "mailbox", "已删除本地邮箱 "+email)
	return s.saveLocked()
}

func (s *Store) ClaimAvailableMailbox(note string) (domain.Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Mailboxes {
		mailbox := &s.state.Mailboxes[i]
		if !mailbox.APIActive || !mailbox.ICloudActive || mailbox.Status != domain.StatusAvailable {
			continue
		}
		mailbox.Status = domain.StatusUsed
		if strings.TrimSpace(note) != "" {
			mailbox.Note = strings.TrimSpace(note)
		}
		mailbox.UpdatedAt = time.Now()
		return *mailbox, s.saveLocked()
	}
	return domain.Mailbox{}, errors.New("没有可用隐私邮箱")
}

func (s *Store) UpsertMessage(mailboxID, remoteID, source, subject, from, body string, receivedAt time.Time) (domain.Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.mailboxIndexLocked(mailboxID)
	if index < 0 {
		return domain.Message{}, false, errors.New("邮箱不存在")
	}
	for _, message := range s.state.Messages {
		if remoteID != "" && message.MailboxID == mailboxID && message.RemoteID == remoteID {
			return message, false, nil
		}
	}
	now := time.Now()
	if receivedAt.IsZero() {
		receivedAt = now
	}
	message := domain.Message{
		ID:         s.nextIDLocked("msg"),
		OwnerID:    s.state.Mailboxes[index].OwnerID,
		MailboxID:  mailboxID,
		RemoteID:   strings.TrimSpace(remoteID),
		Source:     strings.TrimSpace(source),
		Subject:    strings.TrimSpace(subject),
		From:       strings.TrimSpace(from),
		Body:       body,
		ReceivedAt: receivedAt,
		CreatedAt:  now,
	}
	s.state.Messages = append(s.state.Messages, message)
	s.state.Mailboxes[index].ReceiveCount++
	s.state.Mailboxes[index].LastSyncAt = now
	s.state.Mailboxes[index].UpdatedAt = now
	return message, true, s.saveLocked()
}

func (s *Store) MessagesForMailbox(mailboxID string) []domain.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Message, 0)
	for _, message := range s.state.Messages {
		if message.MailboxID == strings.TrimSpace(mailboxID) {
			out = append(out, message)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt.After(out[j].ReceivedAt) })
	return out
}

func (s *Store) DeleteMailboxMessagesByRemoteIDs(mailboxID string, remoteIDs []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mailboxID = strings.TrimSpace(mailboxID)
	index := s.mailboxIndexLocked(mailboxID)
	if index < 0 {
		return 0, errors.New("邮箱不存在")
	}
	targets := make(map[string]bool, len(remoteIDs))
	for _, remoteID := range remoteIDs {
		if remoteID = strings.TrimSpace(remoteID); remoteID != "" {
			targets[remoteID] = true
		}
	}
	if len(targets) == 0 {
		return 0, nil
	}

	mailbox := &s.state.Mailboxes[index]
	next := make([]domain.Message, 0, len(s.state.Messages))
	removed := 0
	remaining := 0
	for _, message := range s.state.Messages {
		if message.MailboxID == mailboxID && targets[strings.TrimSpace(message.RemoteID)] {
			removed++
			if mailbox.LastCodeMessageID == message.ID {
				mailbox.LastCodeMessageID = ""
				mailbox.LastCodeAt = time.Time{}
			}
			continue
		}
		if message.MailboxID == mailboxID {
			remaining++
		}
		next = append(next, message)
	}
	if removed == 0 {
		return 0, nil
	}
	s.state.Messages = next
	mailbox.ReceiveCount = remaining
	mailbox.UpdatedAt = time.Now()
	s.appendEventLocked("info", "mailbox", fmt.Sprintf("已清理本地邮件缓存 %s：%d 封", mailbox.Email, removed))
	return removed, s.saveLocked()
}

func (s *Store) SetMailboxSyncCursor(id string, syncedAt time.Time, lastUID string) (domain.Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.mailboxIndexLocked(id)
	if index < 0 {
		return domain.Mailbox{}, errors.New("邮箱不存在")
	}
	mailbox := &s.state.Mailboxes[index]
	mailbox.LastSyncAt = syncedAt
	mailbox.LastSyncUID = strings.TrimSpace(lastUID)
	mailbox.UpdatedAt = time.Now()
	return *mailbox, s.saveLocked()
}

func (s *Store) SetMailboxLastCode(id, messageID string, servedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.mailboxIndexLocked(id)
	if index < 0 {
		return errors.New("邮箱不存在")
	}
	s.state.Mailboxes[index].LastCodeMessageID = strings.TrimSpace(messageID)
	s.state.Mailboxes[index].LastCodeAt = servedAt
	s.state.Mailboxes[index].UpdatedAt = time.Now()
	return s.saveLocked()
}

func (s *Store) mailboxIndexLocked(id string) int {
	id = strings.TrimSpace(id)
	for i := range s.state.Mailboxes {
		if s.state.Mailboxes[i].ID == id {
			return i
		}
	}
	return -1
}

func randomAPIToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
