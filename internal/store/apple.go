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
	var account domain.AppleAccount
	found, err := s.readEntity("apple_accounts", strings.TrimSpace(id), &account)
	return account, found && err == nil
}

func (s *Store) ICloudSessionByAccountID(accountID string) (domain.ICloudSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var session domain.ICloudSession
	found, err := s.readEntity("icloud_sessions", strings.TrimSpace(accountID), &session)
	return cloneICloudSession(session), found && err == nil
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
	tx, err := s.db.Begin()
	if err != nil {
		return domain.ICloudSession{}, err
	}
	level = strings.TrimSpace(level)
	if level == "" {
		level = "info"
	}
	accountID := strings.TrimSpace(session.AccountID)
	var account domain.AppleAccount
	accountCreated := false
	if accountID != "" {
		if found, err := s.readEntityTx(tx, "apple_accounts", accountID, &account); err != nil || !found {
			_ = tx.Rollback()
			if err != nil {
				return domain.ICloudSession{}, err
			}
			return domain.ICloudSession{}, errors.New("Apple 账号不存在")
		}
	} else {
		accountID, err = s.nextIDTx(tx, "acc")
		if err != nil {
			_ = tx.Rollback()
			return domain.ICloudSession{}, err
		}
		now := time.Now()
		account = domain.AppleAccount{ID: accountID, Label: firstNonEmpty(session.AppleID, "Apple 账号"), AppleID: strings.TrimSpace(session.AppleID), Status: domain.StatusActive, ICloudStatus: iCloudStatusFromSession(session), Note: session.Note, CreatedAt: now, UpdatedAt: now}
		session.AccountID = accountID
		accountCreated = true
	}
	if session.SavedAt.IsZero() {
		session.SavedAt = time.Now()
	}
	if session.OwnerID == "" {
		var adminData []byte
		var admin domain.Admin
		if tx.QueryRow(`SELECT data_json FROM admins LIMIT 1`).Scan(&adminData) == nil && s.decodeEntity("admins", adminData, &admin) == nil {
			session.OwnerID = admin.ID
		}
	}
	var existing domain.ICloudSession
	exists, err := s.readEntityTx(tx, "icloud_sessions", accountID, &existing)
	if err != nil {
		_ = tx.Rollback()
		return domain.ICloudSession{}, err
	}
	if exists {
		session = mergeICloudSession(existing, session)
	}
	session.AccountID = accountID
	account.OwnerID = firstNonEmpty(account.OwnerID, session.OwnerID)
	account.AppleID = firstNonEmpty(session.AppleID, account.AppleID)
	if account.Label == "" {
		account.Label = firstNonEmpty(account.AppleID, "Apple 账号")
	}
	account.Status = domain.StatusActive
	account.ICloudStatus = iCloudStatusFromSession(session)
	account.Note = firstNonEmpty(session.Note, account.Note)
	account.UpdatedAt = time.Now()
	changes := []Change{}
	accountChange, accountChanged, err := s.upsertEntityTx(tx, "apple_accounts", "apple-account", account.ID, account)
	if err != nil {
		_ = tx.Rollback()
		return domain.ICloudSession{}, err
	}
	if accountChanged {
		changes = append(changes, accountChange)
	}
	sessionChange, sessionChanged, err := s.upsertEntityTx(tx, "icloud_sessions", "apple-session", accountID, session)
	if err != nil {
		_ = tx.Rollback()
		return domain.ICloudSession{}, err
	}
	if sessionChanged {
		changes = append(changes, sessionChange)
	}
	if !accountChanged && !sessionChanged {
		_ = tx.Rollback()
		return cloneICloudSession(session), nil
	}
	message := updateMessage
	if accountCreated || !exists {
		message = createMessage
	}
	eventChange, err := s.appendEventTx(tx, level, "apple", message)
	if err != nil {
		_ = tx.Rollback()
		return domain.ICloudSession{}, err
	}
	changes = append(changes, eventChange)
	return cloneICloudSession(session), s.commitTx(tx, changes)
}

// DeleteAppleAccount 删除数据库中的 Apple 账号、登录态、关联邮箱、租约和邮件。
func (s *Store) DeleteAppleAccount(id string) (DeleteAppleAccountResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return DeleteAppleAccountResult{}, err
	}
	var account domain.AppleAccount
	if found, err := s.readEntityTx(tx, "apple_accounts", id, &account); err != nil || !found {
		_ = tx.Rollback()
		if err != nil {
			return DeleteAppleAccountResult{}, err
		}
		return DeleteAppleAccountResult{}, errors.New("Apple 账号不存在")
	}
	result := DeleteAppleAccountResult{AccountID: account.ID, AppleID: account.AppleID}
	_ = tx.QueryRow(`SELECT COUNT(*) FROM mailboxes WHERE json_extract(data_json, '$.account_id') = ?`, id).Scan(&result.Mailboxes)
	_ = tx.QueryRow(`SELECT COUNT(*) FROM messages WHERE json_extract(data_json, '$.mailbox_id') IN (SELECT id FROM mailboxes WHERE json_extract(data_json, '$.account_id') = ?)`, id).Scan(&result.Messages)
	_ = tx.QueryRow(`SELECT COUNT(*) FROM icloud_sessions WHERE id = ?`, id).Scan(&result.ICloudSessions)
	if _, err := tx.Exec(`DELETE FROM messages WHERE json_extract(data_json, '$.mailbox_id') IN (SELECT id FROM mailboxes WHERE json_extract(data_json, '$.account_id') = ?)`, id); err != nil {
		_ = tx.Rollback()
		return DeleteAppleAccountResult{}, err
	}
	if _, err := tx.Exec(`DELETE FROM mailbox_leases WHERE json_extract(data_json, '$.mailbox_id') IN (SELECT id FROM mailboxes WHERE json_extract(data_json, '$.account_id') = ?)`, id); err != nil {
		_ = tx.Rollback()
		return DeleteAppleAccountResult{}, err
	}
	if _, err := tx.Exec(`DELETE FROM mailboxes WHERE json_extract(data_json, '$.account_id') = ?`, id); err != nil {
		_ = tx.Rollback()
		return DeleteAppleAccountResult{}, err
	}
	_, _ = tx.Exec(`DELETE FROM icloud_sessions WHERE id = ?`, id)
	change, _, err := s.deleteEntityTx(tx, "apple_accounts", "apple-account", id)
	if err != nil {
		_ = tx.Rollback()
		return DeleteAppleAccountResult{}, err
	}
	eventChange, err := s.appendEventTx(tx, "warning", "apple", "已删除 Apple 账号及其全部本地关联数据 "+firstNonEmpty(account.AppleID, account.ID))
	if err != nil {
		_ = tx.Rollback()
		return DeleteAppleAccountResult{}, err
	}
	return result, s.commitTx(tx, []Change{change, eventChange})
}

func (s *Store) UpdateICloudSession(accountID string, update func(*domain.ICloudSession) error) (domain.ICloudSession, error) {
	accountID = strings.TrimSpace(accountID)
	session, ok := s.ICloudSessionByAccountID(accountID)
	if !ok {
		return domain.ICloudSession{}, errors.New("Apple 账号登录态不存在")
	}
	if err := update(&session); err != nil {
		return domain.ICloudSession{}, err
	}
	return s.SaveICloudSession(session)
}

func iCloudStatusFromSession(session domain.ICloudSession) string {
	if !session.LastCheckOK && !session.LastCheckedAt.IsZero() {
		return domain.ICloudStatusFailed
	}
	if session.IsICloudPlus && session.CanCreateHME {
		return domain.ICloudStatusActive
	}
	if !session.IsICloudPlus {
		return domain.ICloudStatusNoICloudPlus
	}
	return domain.ICloudStatusActive
}

func sameICloudSessionIdentity(left, right domain.ICloudSession) bool {
	if left.AccountID != "" && right.AccountID != "" {
		return left.AccountID == right.AccountID
	}
	if left.DSID != "" && right.DSID != "" {
		return left.DSID == right.DSID
	}
	return strings.EqualFold(strings.TrimSpace(left.AppleID), strings.TrimSpace(right.AppleID))
}

func mergeICloudSession(existing, incoming domain.ICloudSession) domain.ICloudSession {
	out := cloneICloudSession(incoming)
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
