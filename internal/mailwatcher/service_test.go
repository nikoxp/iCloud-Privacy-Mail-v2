package mailwatcher

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"icloud-privacy-mail-v2/internal/config"
	"icloud-privacy-mail-v2/internal/store"
)

func TestWatcherStatusPersistsAndPublishes(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("创建 SQLite 数据库失败：%v", err)
	}
	defer database.Close()
	changes, unsubscribe := database.SubscribeChanges(8)
	defer unsubscribe()
	service := NewService(config.Default(), database, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.updateStatus(func(status *Status) {
		status.Running = true
		status.Enabled = true
		status.GroupCount = 2
	})
	var persisted Status
	if found, err := database.LoadRuntimeState("mailwatcher", &persisted); err != nil || !found {
		t.Fatalf("读取监听状态失败：found=%t err=%v", found, err)
	}
	if !persisted.Running || persisted.GroupCount != 2 {
		t.Fatalf("监听状态持久化不正确：%+v", persisted)
	}
	select {
	case change := <-changes:
		if change.Resource != "mailwatcher" {
			t.Fatalf("实时资源不正确：%+v", change)
		}
	default:
		t.Fatal("监听状态变化未发布 SSE")
	}
}
