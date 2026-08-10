package protocol

import (
	"net/http"
	"testing"
)

func TestAppleAccountEmptyUnauthorizedResponseIsAuthFailure(t *testing.T) {
	err := appleAccountAPIError(http.StatusUnauthorized, nil, "刷新管理 token")
	code, message, retryable := ErrorDetails(err)
	if code != "apple_account_auth_failed" || !retryable {
		t.Fatalf("空响应 401 的错误分类为 code=%q retryable=%t，期望登录态失效", code, retryable)
	}
	if message == "" {
		t.Fatal("空响应 401 缺少错误说明")
	}
}
