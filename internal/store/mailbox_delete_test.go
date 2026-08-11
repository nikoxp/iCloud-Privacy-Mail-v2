package store

import (
	"path/filepath"
	"testing"
	"time"

	"icloud-privacy-mail-v2/internal/domain"
)

func TestDeleteMailboxMessagesClearsOnlyTargetMailbox(t *testing.T) {
	state, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("创建临时状态失败：%v", err)
	}
	target, _, err := state.UpsertMailboxFromRemote("account_fixture", domain.RemoteMailbox{
		Email:    "delete-target@icloud.com",
		Label:    "delete_target",
		IsActive: true,
	}, "删除测试邮箱")
	if err != nil {
		t.Fatalf("创建待清理邮箱失败：%v", err)
	}
	other, _, err := state.UpsertMailboxFromRemote("account_fixture", domain.RemoteMailbox{
		Email:    "delete-other@icloud.com",
		Label:    "delete_other",
		IsActive: true,
	}, "其他测试邮箱")
	if err != nil {
		t.Fatalf("创建其他邮箱失败：%v", err)
	}

	first, _, err := state.UpsertMessage(target.ID, "icloud:Inbox:101", "icloud", "验证码 101", "sender@example.com", "101", time.Now())
	if err != nil {
		t.Fatalf("保存第一封测试邮件失败：%v", err)
	}
	if _, _, err := state.UpsertMessage(target.ID, "", "local", "本地邮件", "sender@example.com", "local", time.Now()); err != nil {
		t.Fatalf("保存第二封测试邮件失败：%v", err)
	}
	if _, _, err := state.UpsertMessage(other.ID, "icloud:Inbox:202", "icloud", "保留邮件", "sender@example.com", "202", time.Now()); err != nil {
		t.Fatalf("保存其他邮箱邮件失败：%v", err)
	}
	if err := state.SetMailboxLastCode(target.ID, first.ID, time.Now()); err != nil {
		t.Fatalf("保存最近验证码邮件失败：%v", err)
	}

	removed, err := state.DeleteMailboxMessages(target.ID)
	if err != nil {
		t.Fatalf("清空本地邮件失败：%v", err)
	}
	if removed != 2 {
		t.Fatalf("清理数量不正确：得到 %d，期望 2", removed)
	}
	if messages := state.MessagesForMailbox(target.ID); len(messages) != 0 {
		t.Fatalf("目标邮箱仍有本地邮件：%d 封", len(messages))
	}
	if messages := state.MessagesForMailbox(other.ID); len(messages) != 1 {
		t.Fatalf("其他邮箱邮件被误删：剩余 %d 封", len(messages))
	}
	stored, ok := state.FindMailboxByID(target.ID)
	if !ok {
		t.Fatal("清理邮件时不应删除邮箱记录")
	}
	if stored.ReceiveCount != 0 || stored.LastCodeMessageID != "" || !stored.LastCodeAt.IsZero() {
		t.Fatalf("邮箱邮件元数据未清空：%+v", stored)
	}
}
