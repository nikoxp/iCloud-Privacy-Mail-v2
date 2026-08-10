package store

import (
	"path/filepath"
	"testing"

	"icloud-privacy-mail-v2/internal/domain"
)

func TestSetMailboxStatusHandlesOptionalNote(t *testing.T) {
	state, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("创建临时状态失败：%v", err)
	}
	mailbox, _, err := state.UpsertMailboxFromRemote("account_fixture", domain.RemoteMailbox{
		Email: "fixture@icloud.com",
		Label: "x_1",
		Note:  "原备注",
	}, "原备注")
	if err != nil {
		t.Fatalf("创建测试邮箱失败：%v", err)
	}

	newNote := "新备注"
	updated, err := state.SetMailboxStatus(mailbox.ID, nil, nil, domain.StatusUsed, &newNote)
	if err != nil {
		t.Fatalf("更新邮箱状态和备注失败：%v", err)
	}
	if updated.Note != "新备注" {
		t.Fatalf("邮箱备注未更新：%q", updated.Note)
	}

	updated, err = state.SetMailboxStatus(mailbox.ID, nil, nil, domain.StatusAvailable, nil)
	if err != nil {
		t.Fatalf("仅更新邮箱状态失败：%v", err)
	}
	if updated.Note != "新备注" {
		t.Fatalf("仅更新状态时备注发生变化：%q", updated.Note)
	}

	emptyNote := ""
	updated, err = state.SetMailboxStatus(mailbox.ID, nil, nil, "", &emptyNote)
	if err != nil {
		t.Fatalf("清空邮箱备注失败：%v", err)
	}
	if updated.Note != "" {
		t.Fatalf("邮箱备注未清空：%q", updated.Note)
	}
}
