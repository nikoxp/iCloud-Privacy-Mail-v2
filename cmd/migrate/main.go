package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"icloud-privacy-mail-v2/internal/domain"
	"icloud-privacy-mail-v2/internal/store"
)

type legacyUser struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	IsAdmin      bool      `json:"is_admin"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastLoginAt  time.Time `json:"last_login_at"`
}

type legacyState struct {
	NextID         int                     `json:"next_id"`
	Users          []legacyUser            `json:"users"`
	Accounts       []domain.AppleAccount   `json:"accounts"`
	Mailboxes      []domain.Mailbox        `json:"mailboxes"`
	Messages       []domain.Message        `json:"messages"`
	ICloudSession  *domain.ICloudSession   `json:"icloud_session"`
	ICloudSessions []domain.ICloudSession  `json:"icloud_sessions"`
	CreateSettings []domain.CreateSettings `json:"create_settings"`
}

func main() {
	source := flag.String("source", "", "旧项目 state.json 路径")
	target := flag.String("target", "data/state.json", "新项目 state.json 路径")
	force := flag.Bool("force", false, "允许覆盖已存在的新项目状态文件")
	flag.Parse()
	if strings.TrimSpace(*source) == "" {
		fmt.Fprintln(os.Stderr, "请使用 -source 指定旧项目 state.json")
		os.Exit(2)
	}
	if _, err := os.Stat(*target); err == nil && !*force {
		fmt.Fprintln(os.Stderr, "目标状态文件已存在；确认后使用 -force 覆盖")
		os.Exit(2)
	}
	data, err := os.ReadFile(*source)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取旧状态文件失败："+err.Error())
		os.Exit(1)
	}
	var legacy legacyState
	if err := json.Unmarshal(data, &legacy); err != nil {
		fmt.Fprintln(os.Stderr, "解析旧状态文件失败："+err.Error())
		os.Exit(1)
	}
	now := time.Now()
	nextID := legacy.NextID
	if nextID <= 0 {
		nextID = 1
	}
	next := domain.State{
		SchemaVersion:  domain.SchemaVersion,
		NextID:         nextID,
		AppleAccounts:  legacy.Accounts,
		Mailboxes:      legacy.Mailboxes,
		Messages:       legacy.Messages,
		Settings:       domain.DefaultSettings(),
		CreateSettings: domain.DefaultCreateSettings(),
		ICloudSessions: append([]domain.ICloudSession(nil), legacy.ICloudSessions...),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if legacy.ICloudSession != nil {
		next.ICloudSessions = append(next.ICloudSessions, *legacy.ICloudSession)
	}
	if admin, ok := selectLegacyAdmin(legacy.Users); ok {
		next.Admin = &domain.Admin{
			ID:           admin.ID,
			Username:     strings.ToLower(strings.TrimSpace(admin.Username)),
			PasswordHash: admin.PasswordHash,
			CreatedAt:    admin.CreatedAt,
			UpdatedAt:    admin.UpdatedAt,
			LastLoginAt:  admin.LastLoginAt,
		}
		next.CreateSettings = selectLegacyCreateSettings(legacy.CreateSettings, admin.ID)
	} else if len(legacy.CreateSettings) > 0 {
		next.CreateSettings = legacy.CreateSettings[0]
	}
	state, err := store.Open(*target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建新状态文件失败："+err.Error())
		os.Exit(1)
	}
	if err := state.ReplaceState(next); err != nil {
		fmt.Fprintln(os.Stderr, "写入新状态文件失败："+err.Error())
		os.Exit(1)
	}
	orphanMailboxes := countOrphanMailboxes(next.AppleAccounts, next.Mailboxes)
	fmt.Printf("迁移完成：Apple 账号 %d 个，登录态 %d 个，邮箱 %d 个，邮件 %d 封\n", len(next.AppleAccounts), len(next.ICloudSessions), len(next.Mailboxes), len(next.Messages))
	fmt.Printf("迁移报告：旧用户 %d 个，创建配置 %d 份，孤立邮箱 %d 个；旧控制台 Web 登录态未迁移\n", len(legacy.Users), len(legacy.CreateSettings), orphanMailboxes)
	if next.Admin == nil {
		fmt.Println("旧数据没有可用管理员，首次打开新项目时请设置管理员。")
	}
}

func selectLegacyCreateSettings(settings []domain.CreateSettings, ownerID string) domain.CreateSettings {
	for _, item := range settings {
		if strings.TrimSpace(item.OwnerID) == strings.TrimSpace(ownerID) {
			return item
		}
	}
	if len(settings) > 0 {
		return settings[0]
	}
	return domain.DefaultCreateSettings()
}

func countOrphanMailboxes(accounts []domain.AppleAccount, mailboxes []domain.Mailbox) int {
	accountIDs := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		accountIDs[strings.TrimSpace(account.ID)] = struct{}{}
	}
	count := 0
	for _, mailbox := range mailboxes {
		accountID := strings.TrimSpace(mailbox.AccountID)
		if accountID == "" {
			continue
		}
		if _, ok := accountIDs[accountID]; !ok {
			count++
		}
	}
	return count
}

func selectLegacyAdmin(users []legacyUser) (legacyUser, bool) {
	for _, user := range users {
		if user.IsAdmin && strings.TrimSpace(user.Username) != "" && strings.TrimSpace(user.PasswordHash) != "" {
			return user, true
		}
	}
	for _, user := range users {
		if strings.TrimSpace(user.Username) != "" && strings.TrimSpace(user.PasswordHash) != "" {
			return user, true
		}
	}
	return legacyUser{}, false
}
