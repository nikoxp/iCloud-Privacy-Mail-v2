package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"icloud-privacy-mail-v2/internal/domain"
)

var (
	ErrNoAvailableMailbox   = errors.New("没有可用隐私邮箱")
	ErrLeaseProjectRequired = errors.New("project 不能为空")
	ErrLeaseNotFound        = errors.New("邮箱租约不存在")
	ErrLeaseProjectMismatch = errors.New("邮箱租约不属于当前项目")
	ErrLeaseRequestConflict = errors.New("request_id 已被其他领取请求使用")
	ErrLeaseCommitted       = errors.New("邮箱租约已经提交")
	ErrLeaseReleased        = errors.New("邮箱租约已经释放")
	ErrLeaseExpired         = errors.New("邮箱租约已经过期")
	ErrLeaseBindingConflict = errors.New("邮箱与租约绑定关系不一致")
)

// ClaimMailboxLease 原子执行 available -> reserved，并用 project + request_id 保证领取幂等。
func (s *Store) ClaimMailboxLease(project, purpose, requestID, note string, ttl time.Duration, now time.Time) (domain.Mailbox, domain.MailboxLease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project = normalizeLeaseProject(project)
	if project == "" {
		return domain.Mailbox{}, domain.MailboxLease{}, false, ErrLeaseProjectRequired
	}
	purpose = strings.TrimSpace(purpose)
	requestID = strings.TrimSpace(requestID)
	note = strings.TrimSpace(note)
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if now.IsZero() {
		now = time.Now()
	}
	if expired := s.expireMailboxLeasesLocked(now); expired > 0 {
		if err := s.saveLocked(); err != nil {
			return domain.Mailbox{}, domain.MailboxLease{}, false, err
		}
	}

	if requestID != "" {
		for _, lease := range s.state.MailboxLeases {
			if lease.Project != project || lease.RequestID != requestID {
				continue
			}
			if lease.Purpose != purpose {
				return domain.Mailbox{}, lease, false, ErrLeaseRequestConflict
			}
			mailboxIndex := s.mailboxIndexLocked(lease.MailboxID)
			if mailboxIndex < 0 {
				return domain.Mailbox{}, lease, false, ErrLeaseBindingConflict
			}
			return s.state.Mailboxes[mailboxIndex], lease, false, nil
		}
	}

	for i := range s.state.Mailboxes {
		mailbox := &s.state.Mailboxes[i]
		if !mailbox.APIActive || !mailbox.ICloudActive || mailbox.Status != domain.StatusAvailable || mailbox.ActiveLeaseID != "" {
			continue
		}
		lease := domain.MailboxLease{
			ID:        s.nextIDLocked("lease"),
			MailboxID: mailbox.ID,
			Email:     strings.ToLower(strings.TrimSpace(mailbox.Email)),
			Project:   project,
			Purpose:   purpose,
			RequestID: requestID,
			State:     domain.MailboxLeaseClaimed,
			Note:      note,
			ExpiresAt: now.Add(ttl),
			CreatedAt: now,
			UpdatedAt: now,
		}
		mailbox.Status = domain.StatusReserved
		mailbox.ActiveLeaseID = lease.ID
		if note != "" {
			mailbox.Note = note
		}
		mailbox.UpdatedAt = now
		s.state.MailboxLeases = append(s.state.MailboxLeases, lease)
		s.appendEventLocked("info", "mailbox_lease", fmt.Sprintf("项目 %s 已预留邮箱 %s，租约 %s", project, mailbox.Email, lease.ID))
		return *mailbox, lease, true, s.saveLocked()
	}
	return domain.Mailbox{}, domain.MailboxLease{}, false, ErrNoAvailableMailbox
}

// CommitMailboxLease 原子执行 reserved -> used；重复提交同一租约返回幂等成功。
func (s *Store) CommitMailboxLease(leaseID, project, note string, now time.Time) (domain.Mailbox, domain.MailboxLease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now()
	}
	leaseIndex, err := s.authorizedLeaseIndexLocked(leaseID, project)
	if err != nil {
		return domain.Mailbox{}, domain.MailboxLease{}, false, err
	}
	lease := &s.state.MailboxLeases[leaseIndex]
	mailboxIndex := s.mailboxIndexLocked(lease.MailboxID)
	if lease.State == domain.MailboxLeaseCommitted {
		if mailboxIndex < 0 {
			return domain.Mailbox{}, *lease, true, ErrLeaseBindingConflict
		}
		return s.state.Mailboxes[mailboxIndex], *lease, true, nil
	}
	if lease.State == domain.MailboxLeaseReleased {
		return domain.Mailbox{}, *lease, false, ErrLeaseReleased
	}
	if lease.State == domain.MailboxLeaseExpired || !lease.ExpiresAt.After(now) {
		s.expireMailboxLeasesLocked(now)
		if err := s.saveLocked(); err != nil {
			return domain.Mailbox{}, *lease, false, err
		}
		return domain.Mailbox{}, *lease, false, ErrLeaseExpired
	}
	if mailboxIndex < 0 {
		return domain.Mailbox{}, *lease, false, ErrLeaseBindingConflict
	}
	mailbox := &s.state.Mailboxes[mailboxIndex]
	if mailbox.ActiveLeaseID != lease.ID || mailbox.Status != domain.StatusReserved {
		return domain.Mailbox{}, *lease, false, ErrLeaseBindingConflict
	}
	note = strings.TrimSpace(note)
	lease.State = domain.MailboxLeaseCommitted
	lease.CommittedAt = now
	lease.UpdatedAt = now
	mailbox.Status = domain.StatusUsed
	mailbox.ActiveLeaseID = ""
	mailbox.UpdatedAt = now
	if note != "" {
		lease.Note = note
		mailbox.Note = note
	}
	s.appendEventLocked("info", "mailbox_lease", fmt.Sprintf("项目 %s 已提交邮箱租约 %s，邮箱 %s 标记为已使用", lease.Project, lease.ID, mailbox.Email))
	return *mailbox, *lease, false, s.saveLocked()
}

// ReleaseMailboxLease 原子执行 reserved -> available；重复释放或已自动过期均返回幂等成功。
func (s *Store) ReleaseMailboxLease(leaseID, project, note string, now time.Time) (domain.Mailbox, domain.MailboxLease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now()
	}
	leaseIndex, err := s.authorizedLeaseIndexLocked(leaseID, project)
	if err != nil {
		return domain.Mailbox{}, domain.MailboxLease{}, false, err
	}
	lease := &s.state.MailboxLeases[leaseIndex]
	mailboxIndex := s.mailboxIndexLocked(lease.MailboxID)
	if lease.State == domain.MailboxLeaseCommitted {
		return domain.Mailbox{}, *lease, false, ErrLeaseCommitted
	}
	if lease.State == domain.MailboxLeaseReleased || lease.State == domain.MailboxLeaseExpired {
		if mailboxIndex < 0 {
			return domain.Mailbox{}, *lease, true, ErrLeaseBindingConflict
		}
		return s.state.Mailboxes[mailboxIndex], *lease, true, nil
	}
	if !lease.ExpiresAt.After(now) {
		s.expireMailboxLeasesLocked(now)
		if saveErr := s.saveLocked(); saveErr != nil {
			return domain.Mailbox{}, *lease, false, saveErr
		}
		if mailboxIndex < 0 {
			return domain.Mailbox{}, *lease, true, ErrLeaseBindingConflict
		}
		return s.state.Mailboxes[mailboxIndex], *lease, true, nil
	}
	if mailboxIndex < 0 {
		return domain.Mailbox{}, *lease, false, ErrLeaseBindingConflict
	}
	mailbox := &s.state.Mailboxes[mailboxIndex]
	if mailbox.ActiveLeaseID != lease.ID || mailbox.Status != domain.StatusReserved {
		return domain.Mailbox{}, *lease, false, ErrLeaseBindingConflict
	}
	note = strings.TrimSpace(note)
	lease.State = domain.MailboxLeaseReleased
	lease.ReleasedAt = now
	lease.UpdatedAt = now
	mailbox.Status = domain.StatusAvailable
	mailbox.ActiveLeaseID = ""
	mailbox.UpdatedAt = now
	if note != "" {
		lease.Note = note
		mailbox.Note = note
	}
	s.appendEventLocked("info", "mailbox_lease", fmt.Sprintf("项目 %s 已释放邮箱租约 %s，邮箱 %s 恢复可用", lease.Project, lease.ID, mailbox.Email))
	return *mailbox, *lease, false, s.saveLocked()
}

// RenewMailboxLease 延长仍处于 claimed 状态的租约，适合等待人工审核的注册任务。
func (s *Store) RenewMailboxLease(leaseID, project, note string, ttl time.Duration, now time.Time) (domain.Mailbox, domain.MailboxLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if now.IsZero() {
		now = time.Now()
	}
	leaseIndex, err := s.authorizedLeaseIndexLocked(leaseID, project)
	if err != nil {
		return domain.Mailbox{}, domain.MailboxLease{}, err
	}
	lease := &s.state.MailboxLeases[leaseIndex]
	if lease.State == domain.MailboxLeaseCommitted {
		return domain.Mailbox{}, *lease, ErrLeaseCommitted
	}
	if lease.State == domain.MailboxLeaseReleased {
		return domain.Mailbox{}, *lease, ErrLeaseReleased
	}
	if lease.State == domain.MailboxLeaseExpired || !lease.ExpiresAt.After(now) {
		s.expireMailboxLeasesLocked(now)
		if err := s.saveLocked(); err != nil {
			return domain.Mailbox{}, *lease, err
		}
		return domain.Mailbox{}, *lease, ErrLeaseExpired
	}
	mailboxIndex := s.mailboxIndexLocked(lease.MailboxID)
	if mailboxIndex < 0 {
		return domain.Mailbox{}, *lease, ErrLeaseBindingConflict
	}
	mailbox := &s.state.Mailboxes[mailboxIndex]
	if mailbox.ActiveLeaseID != lease.ID || mailbox.Status != domain.StatusReserved {
		return domain.Mailbox{}, *lease, ErrLeaseBindingConflict
	}
	note = strings.TrimSpace(note)
	lease.ExpiresAt = now.Add(ttl)
	lease.UpdatedAt = now
	mailbox.UpdatedAt = now
	if note != "" {
		lease.Note = note
		mailbox.Note = note
	}
	s.appendEventLocked("info", "mailbox_lease", fmt.Sprintf("项目 %s 已续期邮箱租约 %s，新到期时间 %s", lease.Project, lease.ID, lease.ExpiresAt.Format(time.RFC3339)))
	return *mailbox, *lease, s.saveLocked()
}

// SetMailboxLeaseNote 更新租约备注；当前租约或刚提交的租约会同步更新邮箱备注。
func (s *Store) SetMailboxLeaseNote(leaseID, project, note string, now time.Time) (domain.Mailbox, domain.MailboxLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now()
	}
	leaseIndex, err := s.authorizedLeaseIndexLocked(leaseID, project)
	if err != nil {
		return domain.Mailbox{}, domain.MailboxLease{}, err
	}
	lease := &s.state.MailboxLeases[leaseIndex]
	lease.Note = strings.TrimSpace(note)
	lease.UpdatedAt = now
	mailboxIndex := s.mailboxIndexLocked(lease.MailboxID)
	if mailboxIndex < 0 {
		return domain.Mailbox{}, *lease, ErrLeaseBindingConflict
	}
	mailbox := &s.state.Mailboxes[mailboxIndex]
	if mailbox.ActiveLeaseID == lease.ID || (mailbox.ActiveLeaseID == "" && mailbox.Status == domain.StatusUsed && lease.State == domain.MailboxLeaseCommitted) {
		mailbox.Note = lease.Note
		mailbox.UpdatedAt = now
	}
	s.appendEventLocked("info", "mailbox_lease", fmt.Sprintf("项目 %s 已更新邮箱租约 %s 的备注", lease.Project, lease.ID))
	return *mailbox, *lease, s.saveLocked()
}

func (s *Store) FindMailboxLease(leaseID string) (domain.MailboxLease, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	leaseID = strings.TrimSpace(leaseID)
	for _, lease := range s.state.MailboxLeases {
		if lease.ID == leaseID {
			return lease, true
		}
	}
	return domain.MailboxLease{}, false
}

// LatestMailboxLeaseByEmailProject 为旧版按邮箱提交/释放接口提供兼容定位。
func (s *Store) LatestMailboxLeaseByEmailProject(email, project string) (domain.MailboxLease, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	email = strings.ToLower(strings.TrimSpace(email))
	project = normalizeLeaseProject(project)
	var latest domain.MailboxLease
	found := false
	for _, lease := range s.state.MailboxLeases {
		if !strings.EqualFold(lease.Email, email) || lease.Project != project {
			continue
		}
		if !found || lease.CreatedAt.After(latest.CreatedAt) {
			latest = lease
			found = true
		}
	}
	return latest, found
}

func (s *Store) MailboxLeases() []domain.MailboxLease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]domain.MailboxLease(nil), s.state.MailboxLeases...)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// ExpireMailboxLeases 回收超时租约；只释放仍绑定相同 active_lease_id 的邮箱。
func (s *Store) ExpireMailboxLeases(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now()
	}
	count := s.expireMailboxLeasesLocked(now)
	if count == 0 {
		return 0, nil
	}
	return count, s.saveLocked()
}

func (s *Store) expireMailboxLeasesLocked(now time.Time) int {
	count := 0
	for i := range s.state.MailboxLeases {
		lease := &s.state.MailboxLeases[i]
		if lease.State != domain.MailboxLeaseClaimed || lease.ExpiresAt.After(now) {
			continue
		}
		lease.State = domain.MailboxLeaseExpired
		lease.ExpiredAt = now
		lease.UpdatedAt = now
		if mailboxIndex := s.mailboxIndexLocked(lease.MailboxID); mailboxIndex >= 0 {
			mailbox := &s.state.Mailboxes[mailboxIndex]
			if mailbox.ActiveLeaseID == lease.ID {
				mailbox.ActiveLeaseID = ""
				if mailbox.Status == domain.StatusReserved {
					mailbox.Status = domain.StatusAvailable
				}
				mailbox.UpdatedAt = now
			}
		}
		s.appendEventLocked("warning", "mailbox_lease", fmt.Sprintf("邮箱租约 %s 已过期，邮箱 %s 已自动回收", lease.ID, lease.Email))
		count++
	}
	return count
}

func (s *Store) authorizedLeaseIndexLocked(leaseID, project string) (int, error) {
	leaseID = strings.TrimSpace(leaseID)
	project = normalizeLeaseProject(project)
	if project == "" {
		return -1, ErrLeaseProjectRequired
	}
	for i := range s.state.MailboxLeases {
		if s.state.MailboxLeases[i].ID != leaseID {
			continue
		}
		if s.state.MailboxLeases[i].Project != project {
			return -1, ErrLeaseProjectMismatch
		}
		return i, nil
	}
	return -1, ErrLeaseNotFound
}

func (s *Store) normalizeMailboxLeasesLocked(now time.Time) {
	mailboxIndexes := make(map[string]int, len(s.state.Mailboxes))
	for i := range s.state.Mailboxes {
		mailbox := &s.state.Mailboxes[i]
		mailbox.ActiveLeaseID = strings.TrimSpace(mailbox.ActiveLeaseID)
		mailboxIndexes[mailbox.ID] = i
	}
	claimed := make(map[string]int)
	for i := range s.state.MailboxLeases {
		lease := &s.state.MailboxLeases[i]
		lease.ID = strings.TrimSpace(lease.ID)
		lease.MailboxID = strings.TrimSpace(lease.MailboxID)
		lease.Email = strings.ToLower(strings.TrimSpace(lease.Email))
		lease.Project = normalizeLeaseProject(lease.Project)
		lease.Purpose = strings.TrimSpace(lease.Purpose)
		lease.RequestID = strings.TrimSpace(lease.RequestID)
		lease.Note = strings.TrimSpace(lease.Note)
		if lease.CreatedAt.IsZero() {
			lease.CreatedAt = now
		}
		if lease.UpdatedAt.IsZero() {
			lease.UpdatedAt = lease.CreatedAt
		}
		if lease.State != domain.MailboxLeaseClaimed {
			continue
		}
		mailboxIndex, ok := mailboxIndexes[lease.MailboxID]
		if !ok || s.state.Mailboxes[mailboxIndex].ActiveLeaseID != lease.ID || !lease.ExpiresAt.After(now) {
			lease.State = domain.MailboxLeaseExpired
			lease.ExpiredAt = now
			lease.UpdatedAt = now
			continue
		}
		claimed[lease.ID] = mailboxIndex
	}
	for i := range s.state.Mailboxes {
		mailbox := &s.state.Mailboxes[i]
		if mailbox.ActiveLeaseID == "" {
			if mailbox.Status == domain.StatusReserved {
				mailbox.Status = domain.StatusAvailable
			}
			continue
		}
		mailboxIndex, ok := claimed[mailbox.ActiveLeaseID]
		if !ok || mailboxIndex != i {
			mailbox.ActiveLeaseID = ""
			if mailbox.Status == domain.StatusReserved {
				mailbox.Status = domain.StatusAvailable
			}
			continue
		}
		mailbox.Status = domain.StatusReserved
	}
}

func normalizeLeaseProject(project string) string {
	return strings.ToLower(strings.TrimSpace(project))
}

func (s *Store) completeActiveLeaseFromAdminLocked(mailbox *domain.Mailbox, desiredStatus string, now time.Time) {
	for i := range s.state.MailboxLeases {
		lease := &s.state.MailboxLeases[i]
		if lease.ID != mailbox.ActiveLeaseID || lease.State != domain.MailboxLeaseClaimed {
			continue
		}
		lease.UpdatedAt = now
		if desiredStatus == domain.StatusUsed {
			lease.State = domain.MailboxLeaseCommitted
			lease.CommittedAt = now
			s.appendEventLocked("warning", "mailbox_lease", fmt.Sprintf("管理员已提交邮箱租约 %s", lease.ID))
		} else {
			lease.State = domain.MailboxLeaseReleased
			lease.ReleasedAt = now
			s.appendEventLocked("warning", "mailbox_lease", fmt.Sprintf("管理员修改邮箱状态并释放租约 %s", lease.ID))
		}
		break
	}
	mailbox.ActiveLeaseID = ""
}

func (s *Store) syncActiveLeaseNoteLocked(mailbox domain.Mailbox, note string, now time.Time) {
	if mailbox.ActiveLeaseID == "" {
		return
	}
	for i := range s.state.MailboxLeases {
		lease := &s.state.MailboxLeases[i]
		if lease.ID == mailbox.ActiveLeaseID && lease.State == domain.MailboxLeaseClaimed {
			lease.Note = strings.TrimSpace(note)
			lease.UpdatedAt = now
			return
		}
	}
}

func (s *Store) closeMailboxLeasesForDeletionLocked(mailboxID string, now time.Time, note string) {
	for i := range s.state.MailboxLeases {
		lease := &s.state.MailboxLeases[i]
		if lease.MailboxID != mailboxID || lease.State != domain.MailboxLeaseClaimed {
			continue
		}
		lease.State = domain.MailboxLeaseReleased
		lease.ReleasedAt = now
		lease.UpdatedAt = now
		if strings.TrimSpace(note) != "" {
			lease.Note = strings.TrimSpace(note)
		}
	}
}
