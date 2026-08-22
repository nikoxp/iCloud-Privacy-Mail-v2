package protocol

import (
	"strings"
	"testing"
)

func TestParseICloudIMAPMessageKeepsHTMLAndPlainText(t *testing.T) {
	raw := strings.Join([]string{
		"From: OpenAI <noreply@example.com>",
		"To: alias@icloud.com",
		"Subject: =?UTF-8?B?6aqM6K+B56CB?=",
		"Content-Type: multipart/alternative; boundary=mail-boundary",
		"",
		"--mail-boundary",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"纯文本验证码：123456",
		"--mail-boundary",
		"Content-Type: text/html; charset=UTF-8",
		"",
		"<html><body><h1>验证码</h1><p>123456</p></body></html>",
		"--mail-boundary--",
		"",
	}, "\r\n")
	message, recipients, ok := parseICloudIMAPMessage(iCloudIMAPFetchedMessage{UID: "42", Raw: []byte(raw)})
	if !ok {
		t.Fatal("完整邮件解析失败")
	}
	if message.ContentType != "text/html" || !strings.Contains(message.HTMLBody, "<h1>验证码</h1>") {
		t.Fatalf("HTML 正文未保留：%+v", message)
	}
	if !strings.Contains(message.Body, "纯文本验证码：123456") {
		t.Fatalf("纯文本正文未保留：%q", message.Body)
	}
	if !strings.Contains(recipients, "alias@icloud.com") {
		t.Fatalf("收件人解析错误：%q", recipients)
	}
}
