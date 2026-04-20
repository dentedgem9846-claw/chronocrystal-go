package presenter

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPresentTextPassesThrough(t *testing.T) {
	stdout := []byte("hello world\nline 2\n")
	result := Present(stdout, "", 0, 50*time.Millisecond, "", Options{})

	if result.IsBinary {
		t.Fatal("text should not be detected as binary")
	}
	if result.Overflow != "" {
		t.Fatal("small output should not overflow")
	}
	want := string(stdout)
	if result.Content != want {
		t.Errorf("Content = %q, want %q", result.Content, want)
	}
}

func TestPresentBinaryDetected(t *testing.T) {
	stdout := []byte("hello\x00world")
	result := Present(stdout, "", 1, 10*time.Millisecond, "/tmp/data.bin", Options{})

	if !result.IsBinary {
		t.Fatal("null bytes should be detected as binary")
	}
	if !strings.Contains(result.Content, "[error] binary file") {
		t.Errorf("Content = %q, should contain binary error message", result.Content)
	}
	if !strings.Contains(result.Content, "cat -b /tmp/data.bin") {
		t.Errorf("Content = %q, should suggest cat -b with path", result.Content)
	}
}

func TestPresentBinaryImage(t *testing.T) {
	stdout := []byte("\x89PNG\r\n\x1a\n") // PNG header with null-like bytes
	// Force binary by injecting a null byte
	stdout = append(stdout, 0x00)
	result := Present(stdout, "", 1, 10*time.Millisecond, "/tmp/photo.png", Options{})

	if !result.IsBinary {
		t.Fatal("should be detected as binary")
	}
	if !strings.Contains(result.Content, "binary image file") {
		t.Errorf("Content = %q, should mention image file", result.Content)
	}
	if !strings.Contains(result.Content, "see /tmp/photo.png") {
		t.Errorf("Content = %q, should suggest 'see' for image", result.Content)
	}
}

func TestPresentOverflowLines(t *testing.T) {
	dir := t.TempDir()

	// Generate 250 lines
	var buf strings.Builder
	for i := 0; i < 250; i++ {
		buf.WriteString("line content here\n")
	}
	stdout := []byte(buf.String())

	result := Present(stdout, "", 0, 100*time.Millisecond, "", Options{
		MaxLines:    200,
		MaxBytes:    1 << 20, // large enough to not trigger byte limit
		OverflowDir: dir,
	})

	if result.IsBinary {
		t.Fatal("text should not be binary")
	}
	if result.Overflow == "" {
		t.Fatal("should have overflow file")
	}
	if !strings.Contains(result.Content, "output truncated") {
		t.Error("should contain truncation notice")
	}
	if !strings.Contains(result.Content, "250 lines") {
		t.Error("should show total line count")
	}
	if !strings.Contains(result.Content, "grep <pattern>") {
		t.Error("should contain exploration hint with grep")
	}
	if !strings.Contains(result.Content, "tail 100") {
		t.Error("should contain exploration hint with tail")
	}

	// Verify overflow file was created with full content
	data, err := filepath.Glob(filepath.Join(dir, "cmd-*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("overflow file should exist")
	}
}

func TestPresentOverflowBytes(t *testing.T) {
	dir := t.TempDir()

	// Generate output > 50KB but few lines
	line := strings.Repeat("a", 1000) + "\n"
	var buf strings.Builder
	for buf.Len() <= 60*1024 {
		buf.WriteString(line)
	}
	stdout := []byte(buf.String())

	result := Present(stdout, "", 0, 100*time.Millisecond, "", Options{
		MaxLines:    10000, // large enough to not trigger line limit
		MaxBytes:   50 * 1024,
		OverflowDir: dir,
	})

	if result.Overflow == "" {
		t.Fatal("should have overflow file due to byte limit")
	}
	if !strings.Contains(result.Content, "output truncated") {
		t.Error("should contain truncation notice")
	}
}

func TestPresentStderrAttached(t *testing.T) {
	stdout := []byte("some output\n")
	result := Present(stdout, "command not found", 127, 50*time.Millisecond, "", Options{})

	if !strings.Contains(result.Content, "[stderr] command not found") {
		t.Errorf("Content = %q, should attach stderr on failure", result.Content)
	}
}

func TestPresentStderrNotAttachedOnSuccess(t *testing.T) {
	stdout := []byte("all good\n")
	result := Present(stdout, "warning: something", 0, 50*time.Millisecond, "", Options{})

	if strings.Contains(result.Content, "[stderr]") {
		t.Errorf("Content = %q, should NOT attach stderr on exit 0", result.Content)
	}
}

func TestPresentMetadataFooter(t *testing.T) {
	// Metadata footer is added by FormatResult, not Present
	stdout := []byte("hello\n")
	result := Present(stdout, "", 0, 45*time.Millisecond, "", Options{})
	formatted := FormatResult(result)

	if !strings.Contains(formatted, "[exit:0 | 45ms]") {
		t.Errorf("FormatResult = %q, should contain metadata footer", formatted)
	}
}

func TestPresentDurationFormatting(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Millisecond, "45ms"},
		{1200 * time.Millisecond, "1.2s"},
		{90 * time.Second, "1m30s"},
		{60 * time.Second, "1m"},
		{500 * time.Millisecond, "500ms"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatResultBinary(t *testing.T) {
	result := Result{
		Content:  "[error] binary file (1.0KB). Use: cat -b /tmp/data.bin for base64",
		IsBinary: true,
		ExitCode: 1,
		Duration: 50 * time.Millisecond,
	}
	formatted := FormatResult(result)

	if !strings.HasPrefix(formatted, "[error] binary file") {
		t.Errorf("should start with binary error, got %q", formatted)
	}
	if !strings.HasSuffix(formatted, "[exit:1 | 50ms]") {
		t.Errorf("should end with metadata footer, got %q", formatted)
	}
}

func TestFormatResultNormal(t *testing.T) {
	result := Result{
		Content:  "hello world",
		ExitCode: 0,
		Duration: 10 * time.Millisecond,
	}
	formatted := FormatResult(result)

	if !strings.Contains(formatted, "hello world") {
		t.Errorf("should contain content, got %q", formatted)
	}
	if !strings.Contains(formatted, "[exit:0 | 10ms]") {
		t.Errorf("should contain footer, got %q", formatted)
	}
}

func TestPresentUTF8Validation(t *testing.T) {
	// Invalid UTF-8 sequence
	stdout := []byte("hello \xff\xfe world")
	result := Present(stdout, "", 1, 10*time.Millisecond, "", Options{})

	if !result.IsBinary {
		t.Fatal("invalid UTF-8 should be detected as binary")
	}
	if !strings.Contains(result.Content, "[error] binary file") {
		t.Errorf("Content = %q, should contain binary error", result.Content)
	}
}

func TestPresentControlCharRatio(t *testing.T) {
	// High ratio of control characters (>10% of non-space chars)
	var buf strings.Builder
	for i := 0; i < 100; i++ {
		buf.WriteByte(0x01) // control char
	}
	buf.WriteString("a") // one normal char
	stdout := []byte(buf.String())

	result := Present(stdout, "", 1, 10*time.Millisecond, "", Options{})
	if !result.IsBinary {
		t.Fatal("high control char ratio should be detected as binary")
	}
}