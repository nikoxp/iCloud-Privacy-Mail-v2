package store

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"icloud-privacy-mail-v2/internal/domain"
)

type DeleteAppleAccountResult struct {
	AccountID      string `json:"account_id"`
	AppleID        string `json:"apple_id"`
	Mailboxes      int    `json:"mailboxes"`
	Messages       int    `json:"messages"`
	ICloudSessions int    `json:"icloud_sessions"`
}

func (s *Store) FindAppleAccount(id string) (domain.AppleAccount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id = strings.TrimSpace(id)
	for _, account := range s.state.AppleAccounts {
		if account.ID == id {
			return account, true
		}
	}
	return domain.AppleAccount{}, false
}

func (s *Store) ICloudSessionByAccountID(accountID string) (domain.ICloudSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accountID = strings.TrimSpace(accountID)
	for _, session := range s.state.ICloudSessions {
		if session.AccountID == accountID {
			return cloneICloudSession(session), true
		}
	}
	return domain.ICloudSession{}, false
}

func (s *Store) SaveICloudSession(session domain.ICloudSession) (domain.ICloudSession, error) {
	return s.saveICloudSession(session, "info", "已更新 Apple 登录态", "已保存 Apple 登录态")
}

func (s *Store) SaveICloudSessionWithEvent(session domain.ICloudSession, level, message string) (domain.ICloudSession, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "已更新 Apple 登录态"
	}
	return s.saveICloudSession(session, level, message, message)
}

func (s *Store) saveICloudSession(session domain.ICloudSession, level, updateMessage, createMessage string) (domain.ICloudSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	level = strings.TrimSpace(level)
	if level == "" {
		level = "info"
	}
	if strings.TrimSpace(session.AccountID) != "" {
		accountExists := false
		for _, account := range s.state.AppleAccounts {
			if account.ID == strings.TrimSpace(session.AccountID) {
				accountExists = true
				break
			}
		}
		if !accountExists {
			return domain.ICloudSession{}, errors.New("Apple 账号不存在")
		}
	}
	if s.state.Admin != nil && strings.TrimSpace(session.OwnerID) == "" {
		session.OwnerID = s.state.Admin.ID
	}
	if session.SavedAt.IsZero() {
		session.SavedAt = time.Now()
	}
	if strings.TrimSpace(session.AccountID) == "" {
		session.AccountID = s.ensureAppleAccountLocked(session)
	} else {
		s.touchAppleAccountLocked(session.AccountID, session)
	}
	for i, existing := range s.state.ICloudSessions {
		if sameICloudSessionIdentity(existing, session) {
			merged := mergeICloudSession(existing, session)
			s.state.ICloudSessions[i] = merged
			s.touchAppleAccountLocked(merged.AccountID, merged)
			s.appendEventLocked(level, "apple", updateMessage)
			return cloneICloudSession(merged), s.saveLocked()
		}
	}
	s.state.ICloudSessions = append(s.state.ICloudSessions, cloneICloudSession(session))
	s.touchAppleAccountLocked(session.AccountID, session)
	s.appendEventLocked(level, "apple", createMessage)
	return cloneICloudSession(session), s.saveLocked()
}

// DeleteAppleAccount 删除本地 Apple 账号及其关联登录态、邮箱和邮件。
func (s *Store) DeleteAppleAccount(id string) (DeleteAppleAccountResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	accountIndex := -1
	var account domain.AppleAccount
	for index, candidate := range s.state.AppleAccounts {
		if candidate.ID == id {
			accountIndex = index
			account = candidate
			break
		}
	}
	if accountIndex < 0 {
		return DeleteAppleAccountResult{}, errors.New("Apple 账号不存在")
	}

	result := DeleteAppleAccountResult{AccountID: account.ID, AppleID: account.AppleID}
	s.state.AppleAccounts = append(s.state.AppleAccounts[:accountIndex], s.state.AppleAccounts[accountIndex+1:]...)

	sessions := s.state.ICloudSessions[:0]
	for _, session := range s.state.ICloudSessions {
		if session.AccountID == id {
			result.ICloudSessions++
			continue
		}
		sessions = append(sessions, session)
	}
	s.state.ICloudSessions = sessions

	deletedMailboxIDs := make(map[string]struct{})
	mailboxes := s.state.Mailboxes[:0]
	for _, mailbox := range s.state.Mailboxes {
		if mailbox.AccountID == id {
			result.Mailboxes++
			deletedMailboxIDs[mailbox.ID] = struct{}{}
			s.closeMailboxLeasesForDeletionLocked(mailbox.ID, time.Now(), "Apple 账号已由管理员删除")
			continue
		}
		mailboxes = append(mailboxes, mailbox)
	}
	s.state.Mailboxes = mailboxes

	messages := s.state.Messages[:0]
	for _, message := range s.state.Messages {
		if _, deleted := deletedMailboxIDs[message.MailboxID]; deleted {
			result.Messages++
			continue
		}
		messages = append(messages, message)
	}
	s.state.Messages = messages

	accountIDs := s.state.CreateSettings.AccountIDs[:0]
	for _, accountID := range s.state.CreateSettings.AccountIDs {
		if accountID != id {
			accountIDs = append(accountIDs, accountID)
		}
	}
	s.state.CreateSettings.AccountIDs = accountIDs
	s.appendEventLocked("warning", "apple", "已删除本地 Apple 账号 "+firstNonEmpty(account.AppleID, account.ID))
	return result, s.saveLocked()
}

func (s *Store) UpdateICloudSession(accountID string, update func(*domain.ICloudSession) error) (domain.ICloudSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	accountID = strings.TrimSpace(accountID)
	for i := range s.state.ICloudSessions {
		if s.state.ICloudSessions[i].AccountID != accountID {
			continue
		}
		next := cloneICloudSession(s.state.ICloudSessions[i])
		if err := update(&next); err != nil {
			return domain.ICloudSession{}, err
		}
		s.state.ICloudSessions[i] = next
		s.touchAppleAccountLocked(accountID, next)
		return cloneICloudSession(next), s.saveLocked()
	}
	return domain.ICloudSession{}, errors.New("Apple 账号登录态不存在")
}

func (s *Store) ensureAppleAccountLocked(session domain.ICloudSession) string {
	appleID := strings.TrimSpace(session.AppleID)
	for i, account := range s.state.AppleAccounts {
		if appleID != "" && strings.EqualFold(account.AppleID, appleID) {
			s.updateAppleAccountFromSessionLocked(i, session)
			return account.ID
		}
	}
	now := time.Now()
	label := appleID
	if label == "" {
		label = "Apple 账号 " + now.Format("0102-150405")
	}
	account := domain.AppleAccount{
		ID:           s.nextIDLocked("acc"),
		OwnerID:      session.OwnerID,
		Label:        label,
		AppleID:      appleID,
		Status:       domain.StatusActive,
		ICloudStatus: iCloudStatusFromSession(session),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.state.AppleAccounts = append(s.state.AppleAccounts, account)
	return account.ID
}

func (s *Store) touchAppleAccountLocked(accountID string, session domain.ICloudSession) {
	for i, account := range s.state.AppleAccounts {
		if account.ID == strings.TrimSpace(accountID) {
			s.updateAppleAccountFromSessionLocked(i, session)
			return
		}
	}
}

func (s *Store) updateAppleAccountFromSessionLocked(index int, session domain.ICloudSession) {
	if index < 0 || index >= len(s.state.AppleAccounts) {
		return
	}
	account := &s.state.AppleAccounts[index]
	if appleID := strings.TrimSpace(session.AppleID); appleID != "" {
		account.AppleID = appleID
		if strings.TrimSpace(account.Label) == "" {
			account.Label = appleID
		}
	}
	account.Status = domain.StatusActive
	account.ICloudStatus = iCloudStatusFromSession(session)
	account.UpdatedAt = time.Now()
}

func iCloudStatusFromSession(session domain.ICloudSession) string {
	if len(session.Cookies) > 0 {
		if !session.IsICloudPlus {
			return domain.ICloudStatusNoICloudPlus
		}
		if session.CanCreateHME {
			return domain.ICloudStatusActive
		}
	}
	for _, state := range session.LoginStates {
		if state.Kind == domain.LoginStateAppleAccount && strings.TrimSpace(state.Scnt) != "" {
			return domain.ICloudStatusActive
		}
		if state.Kind == domain.LoginStateICloudIMAP && strings.TrimSpace(state.IMAPAppPassword) != "" {
			return domain.ICloudStatusActive
		}
	}
	return domain.ICloudStatusNeedLogin
}

func sameICloudSessionIdentity(left, right domain.ICloudSession) bool {
	if left.AccountID != "" && left.AccountID == right.AccountID {
		return true
	}
	if left.DSID != "" && left.DSID == right.DSID {
		return true
	}
	return left.AppleID != "" && strings.EqualFold(left.AppleID, right.AppleID)
}

func mergeICloudSession(existing, incoming domain.ICloudSession) domain.ICloudSession {
	out := incoming
	out.OwnerID = firstNonEmpty(incoming.OwnerID, existing.OwnerID)
	out.AccountID = firstNonEmpty(incoming.AccountID, existing.AccountID)
	if out.SavedAt.IsZero() {
		out.SavedAt = existing.SavedAt
	}
	out.AppleID = firstNonEmpty(incoming.AppleID, existing.AppleID)
	out.DSID = firstNonEmpty(incoming.DSID, existing.DSID)
	out.ClientID = firstNonEmpty(incoming.ClientID, existing.ClientID)
	out.ClientBuildNumber = firstNonEmpty(incoming.ClientBuildNumber, existing.ClientBuildNumber)
	out.MasteringNumber = firstNonEmpty(incoming.MasteringNumber, existing.MasteringNumber)
	out.PremiumMailBaseURL = firstNonEmpty(incoming.PremiumMailBaseURL, existing.PremiumMailBaseURL)
	out.MailGatewayBaseURL = firstNonEmpty(incoming.MailGatewayBaseURL, existing.MailGatewayBaseURL)
	out.MailBaseURL = firstNonEmpty(incoming.MailBaseURL, existing.MailBaseURL)
	out.Host = firstNonEmpty(incoming.Host, existing.Host)
	out.IsICloudPlus = incoming.IsICloudPlus || existing.IsICloudPlus
	out.CanCreateHME = incoming.CanCreateHME || existing.CanCreateHME
	if len(out.Cookies) == 0 {
		out.Cookies = append([]domain.SessionCookie(nil), existing.Cookies...)
	}
	out.LoginStates = mergeLoginStates(existing.LoginStates, incoming.LoginStates)
	out.Note = firstNonEmpty(incoming.Note, existing.Note)
	if out.LastCheckedAt.IsZero() {
		out.LastCheckedAt = existing.LastCheckedAt
	}
	if strings.TrimSpace(out.LastStatusMessage) == "" {
		out.LastStatusMessage = existing.LastStatusMessage
	}
	return out
}

func mergeLoginStates(existing, incoming []domain.LoginState) []domain.LoginState {
	out := cloneLoginStates(existing)
	for _, state := range incoming {
		replaced := false
		for i := range out {
			if out[i].Kind == state.Kind {
				out[i] = cloneLoginState(state)
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, cloneLoginState(state))
		}
	}
	return out
}

func cloneICloudSession(session domain.ICloudSession) domain.ICloudSession {
	data, _ := json.Marshal(session)
	var out domain.ICloudSession
	_ = json.Unmarshal(data, &out)
	return out
}

func cloneLoginStates(states []domain.LoginState) []domain.LoginState {
	out := make([]domain.LoginState, 0, len(states))
	for _, state := range states {
		out = append(out, cloneLoginState(state))
	}
	return out
}

func cloneLoginState(state domain.LoginState) domain.LoginState {
	state.Cookies = append([]domain.SessionCookie(nil), state.Cookies...)
	return state
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
