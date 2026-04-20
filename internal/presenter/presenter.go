package presenter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type Options struct {
	MaxLines    int    // truncate after this many lines (default 200)
	MaxBytes    int    // truncate after this many bytes (default 50KB)
	OverflowDir string // directory for overflow files (default /tmp/cmd-output)
}

type Result struct {
	Content  string // processed content for LLM
	Overflow string // path to overflow file, empty if no overflow
	ExitCode int
	Duration time.Duration
	Stderr   string // attached stderr if non-empty
	IsBinary bool   // set if binary detected
}

const (
	defaultMaxLines    = 200
	defaultMaxBytes    = 50 * 1024
	defaultOverflowDir = "/tmp/cmd-output"
)

func applyDefaults(opts Options) Options {
	if opts.MaxLines <= 0 {
		opts.MaxLines = defaultMaxLines
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultMaxBytes
	}
	if opts.OverflowDir == "" {
		opts.OverflowDir = defaultOverflowDir
	}
	return opts
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".webp": true, ".svg": true,
}

func isImageExt(path string) bool {
	return imageExtensions[strings.ToLower(filepath.Ext(path))]
}

// isBinary checks whether data appears to be binary content.
// Detection: null bytes in first 8KB, invalid UTF-8, or high control-char ratio.
func isBinaryCheck(data []byte) bool {
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	chunk := data[:checkLen]

	// Null byte check
	for _, b := range chunk {
		if b == 0x00 {
			return true
		}
	}

	// UTF-8 validity
	if !utf8.Valid(data) {
		return true
	}

	// Control character ratio: >10% of non-space chars are control chars
	// in 0x00-0x1F range, excluding \t \n \r
	nonSpace := 0
	control := 0
	for _, r := range string(chunk) {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		nonSpace++
		if r >= 0x00 && r <= 0x1F {
			control++
		}
	}
	if nonSpace > 0 && float64(control)/float64(nonSpace) > 0.10 {
		return true
	}

	return false
}

func formatSize(n int) string {
	const kb = 1024
	const mb = kb * 1024
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1fMB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1fKB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

// truncateLines rune-safely truncates to the first n lines of data.
// Returns the truncated content and the total line count.
func truncateLines(data []byte, maxLines int) (string, int) {
	content := string(data)
	lines := strings.Split(content, "\n")
	// If content ends with \n, Split produces a trailing empty string.
	// Adjust: that's not an extra line.
	totalLines := len(lines)
	if totalLines > 0 && lines[totalLines-1] == "" {
		totalLines--
	}
	if totalLines > maxLines {
		truncated := strings.Join(lines[:maxLines], "\n")
		return truncated, totalLines
	}
	return content, totalLines
}

// Present processes raw execution output for LLM consumption.
// It applies: binary guard, overflow mode, stderr attachment, metadata footer.
func Present(stdout []byte, stderr string, exitCode int, duration time.Duration, pathHint string, opts Options) Result {
	opts = applyDefaults(opts)

	if isBinaryCheck(stdout) {
		size := formatSize(len(stdout))
		var msg string
		if isImageExt(pathHint) {
			msg = fmt.Sprintf("[error] binary image file (%s). Use: see %s", size, pathHint)
		} else {
			msg = fmt.Sprintf("[error] binary file (%s). Use: cat -b %s for base64", size, pathHint)
		}
		return Result{
			Content:  msg,
			IsBinary: true,
			ExitCode: exitCode,
			Duration: duration,
			Stderr:   stderr,
		}
	}

	content := string(stdout)
	overflow := ""

	// Overflow check: lines
	truncated, totalLines := truncateLines(stdout, opts.MaxLines)
	truncatedByLines := totalLines > opts.MaxLines

	// Overflow check: bytes
	truncatedByBytes := len(truncated) > opts.MaxBytes
	if truncatedByBytes {
		// Rune-safe byte truncation
		trunc := []byte(truncated)
		for len(trunc) > opts.MaxBytes {
			_, size := utf8.DecodeLastRune(trunc)
			trunc = trunc[:len(trunc)-size]
		}
		truncated = string(trunc)
	}

	if truncatedByLines || truncatedByBytes {
		// Write full output to overflow file
		if err := os.MkdirAll(opts.OverflowDir, 0o755); err == nil {
			ts := time.Now().Format("20060102-150405")
			overflowPath := filepath.Join(opts.OverflowDir, fmt.Sprintf("cmd-%s.txt", ts))
			if err := os.WriteFile(overflowPath, stdout, 0o644); err == nil {
				overflow = overflowPath
			}
		}

		// Build truncation notice
		size := formatSize(len(stdout))
		notice := fmt.Sprintf(
			"\n--- output truncated (%d lines, %s) ---\nFull output: %s\nExplore: cat %s | grep <pattern>\n         cat %s | tail 100",
			totalLines, size, overflow, overflow, overflow,
		)
		content = truncated + notice
	}

	// Stderr attachment: only on failure
	if stderr != "" && exitCode != 0 {
		content += "\n[stderr] " + stderr
	}

	return Result{
		Content:  content,
		Overflow: overflow,
		ExitCode: exitCode,
		Duration: duration,
		Stderr:   stderr,
	}
}

// FormatResult renders a Result into the final string for the LLM.
func FormatResult(r Result) string {
	footer := fmt.Sprintf("\n[exit:%d | %s]", r.ExitCode, formatDuration(r.Duration))

	if r.IsBinary {
		return r.Content + footer
	}

	return r.Content + footer
}