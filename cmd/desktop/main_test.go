package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadEnvSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meshlink.env")
	data := "PORT=4101\nRELAY=false\nBOOTSTRAP_ADDR=1.2.3.4:4001:peer\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	settings, err := readEnvSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Port != "4101" || settings.Relay || settings.Bootstrap != "1.2.3.4:4001:peer" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestReadSystemdSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meshlink.service")
	data := "[Service]\nExecStart=/usr/local/bin/p2p-node -port 4102 -config /etc/meshlink/data -relay\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	settings, err := readSystemdSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Port != "4102" || !settings.Relay || settings.Bootstrap != "" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestReadLaunchDaemonSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "com.meshlink.p2p.plist")
	data := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.meshlink.p2p</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/p2p-node</string>
        <string>-port</string>
        <string>4103</string>
        <string>-config</string>
        <string>/etc/meshlink/data</string>
        <string>-bootstrap</string>
        <string>/ip4/1.2.3.4/tcp/4001/p2p/peer</string>
    </array>
</dict>
</plist>`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	settings, err := readLaunchDaemonSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Port != "4103" || settings.Relay || settings.Bootstrap != "/ip4/1.2.3.4/tcp/4001/p2p/peer" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestDiffLogLinesAppendsOnlyNewTail(t *testing.T) {
	previous := []string{"a", "b", "c"}
	current := []string{"b", "c", "d", "e"}

	got := diffLogLines(previous, current)
	want := []string{"d", "e"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diffLogLines() = %#v, want %#v", got, want)
	}
}

func TestDiffLogLinesAfterClearDoesNotReplaySnapshot(t *testing.T) {
	previous := normalizeLogLines("line1\nline2\n")
	current := normalizeLogLines("line1\nline2\n")

	got := diffLogLines(previous, current)
	if len(got) != 0 {
		t.Fatalf("diffLogLines() = %#v, want empty", got)
	}
}

func TestDiffLogLinesSameSnapshot(t *testing.T) {
	previous := normalizeLogLines("line1\nline2\nline3")
	current := normalizeLogLines("line1\nline2\nline3")

	got := diffLogLines(previous, current)
	if len(got) != 0 {
		t.Fatalf("diffLogLines() = %#v, want empty", got)
	}
}

func TestNormalizeLogLines(t *testing.T) {
	got := normalizeLogLines("\nline1\r\n\nline2\n")
	want := []string{"line1", "line2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeLogLines() = %#v, want %#v", got, want)
	}
}

func TestNormalizeRepeatingLogLineIgnoresTimePrefix(t *testing.T) {
	got := normalizeRepeatingLogLine("12:34:56  [控制] 映射成功: 10.0.0.2 -> peer")
	want := "[控制] 映射成功: 10.0.0.2 -> peer"
	if got != want {
		t.Fatalf("normalizeRepeatingLogLine() = %q, want %q", got, want)
	}
}

func TestNormalizeRepeatingLogLineIgnoresAggregateSuffix(t *testing.T) {
	got := normalizeRepeatingLogLine("[控制] 映射成功: 10.0.0.2 -> peer  |  共 12 次")
	want := "[控制] 映射成功: 10.0.0.2 -> peer"
	if got != want {
		t.Fatalf("normalizeRepeatingLogLine() = %q, want %q", got, want)
	}
}
