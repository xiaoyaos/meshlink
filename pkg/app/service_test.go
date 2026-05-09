package app

import "testing"

func TestDaemonReadyRecognizesChineseReadyLine(t *testing.T) {
	if !isDaemonReady("[就绪] MeshLink 已启动并在后台运行") {
		t.Fatal("expected Chinese ready log line to mark daemon ready")
	}
}

func TestDaemonReadyRecognizesLegacyReadyLines(t *testing.T) {
	lines := []string{
		"[ready] daemon started",
		"[STATE] CONNECTED",
		"Network is ready",
	}

	for _, line := range lines {
		if !isDaemonReady(line) {
			t.Fatalf("expected ready line to be recognized: %s", line)
		}
	}
}
