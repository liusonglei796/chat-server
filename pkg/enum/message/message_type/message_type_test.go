package message_type

import "testing"

// TestMessageTypeValuesStable 锁定持久化到数据库和客户端的消息类型数值。
// 1（原语音）和 3（原音视频通话）已移除，但编号不能被后续类型复用。
func TestMessageTypeValuesStable(t *testing.T) {
	if Text != 0 {
		t.Fatalf("Text = %d, want 0", Text)
	}
	if File != 2 {
		t.Fatalf("File = %d, want 2", File)
	}
	if Recall != 4 {
		t.Fatalf("Recall = %d, want 4", Recall)
	}
	if KickNotification != 99 {
		t.Fatalf("KickNotification = %d, want 99", KickNotification)
	}
}
