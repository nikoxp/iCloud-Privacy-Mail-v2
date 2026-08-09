package protocol

import (
	"context"
	"time"
)

type IMAPSyncResult struct {
	MessagesByMailbox map[string][]ICloudSyncedMessage
	LastUID           string
}

func SyncICloudIMAPMessagesDetailed(ctx context.Context, state LoginState, mailboxes []Mailbox, after time.Time, keyword string, maxMessages int) (IMAPSyncResult, error) {
	result, err := SyncICloudIMAPMessagesWithCursor(ctx, state, mailboxes, after, keyword, maxMessages)
	if err != nil {
		return IMAPSyncResult{}, err
	}
	return IMAPSyncResult{MessagesByMailbox: result.MessagesByMailbox, LastUID: result.LastUID}, nil
}
