package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chronocrystal/chronocrystal-go/internal/presenter"
)

func setupFS(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("WORKSPACE_DIR", dir)
	return dir
}

func TestCatReadFile(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.catHandler(context.Background(), []string{"test.txt"}, "")
	if err != nil {
		t.Fatalf("catHandler error: %v", err)
	}
	if output != "hello world" {
		t.Errorf("catHandler = %q, want %q", output, "hello world")
	}
}

func TestCatNoArgs(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.catHandler(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("catHandler error: %v", err)
	}
	if output != "usage: cat <path> [-b]" {
		t.Errorf("catHandler no args = %q, want usage", output)
	}
}

func TestCatBinaryDetection(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	path := filepath.Join(dir, "binary.bin")
	binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	if err := os.WriteFile(path, binaryData, 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.catHandler(context.Background(), []string{"binary.bin"}, "")
	if err != nil {
		t.Fatalf("catHandler error: %v", err)
	}
	if !strings.Contains(output, "[error]") || !strings.Contains(output, "binary file") {
		t.Errorf("catHandler binary = %q, want binary file error", output)
	}
	if !strings.Contains(output, "cat -b") {
		t.Errorf("catHandler binary = %q, want suggestion to use -b", output)
	}
}

func TestCatBase64Mode(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	path := filepath.Join(dir, "binary.bin")
	binaryData := []byte{0x00, 0x01, 0x02, 0x03}
	if err := os.WriteFile(path, binaryData, 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.catHandler(context.Background(), []string{"-b", "binary.bin"}, "")
	if err != nil {
		t.Fatalf("catHandler error: %v", err)
	}
	// Should return base64-encoded content, not a binary error.
	if strings.Contains(output, "[error]") {
		t.Errorf("catHandler -b = %q, should not contain error", output)
	}
}

func TestCatPathTraversal(t *testing.T) {

	r := NewRegistry(presenter.Options{}, nil, nil)

	_, err := r.catHandler(context.Background(), []string{"../../../etc/passwd"}, "")
	if err == nil {
		t.Error("catHandler should reject path traversal, got nil error")
	}
}

func TestLsDirectory(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	// Create test files.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := r.lsHandler(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("lsHandler error: %v", err)
	}
	if !strings.Contains(output, "subdir/") {
		t.Errorf("lsHandler = %q, want directory listing with subdir/", output)
	}
	if !strings.Contains(output, "a.txt") {
		t.Errorf("lsHandler = %q, want a.txt in listing", output)
	}
}

func TestLsEmptyDir(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	// Create a new empty subdirectory.
	emptyDir := filepath.Join(dir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := r.lsHandler(context.Background(), []string{"empty"}, "")
	if err != nil {
		t.Fatalf("lsHandler error: %v", err)
	}
	if output != "(empty directory)" {
		t.Errorf("lsHandler empty dir = %q, want (empty directory)", output)
	}
}

func TestWriteFile(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	output, err := r.writeHandler(context.Background(), []string{"newfile.txt", "hello"}, "")
	if err != nil {
		t.Fatalf("writeHandler error: %v", err)
	}
	if !strings.Contains(output, "wrote") {
		t.Errorf("writeHandler = %q, want wrote confirmation", output)
	}

	// Verify file exists.
	data, err := os.ReadFile(filepath.Join(dir, "newfile.txt"))
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file content = %q, want %q", string(data), "hello")
	}
}

func TestWriteFromStdin(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	_, err := r.writeHandler(context.Background(), []string{"stdinfile.txt"}, "piped content")
	if err != nil {
		t.Fatalf("writeHandler error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "stdinfile.txt"))
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if string(data) != "piped content" {
		t.Errorf("file content = %q, want %q", string(data), "piped content")
	}
}

func TestWriteCreatesParentDirs(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	output, err := r.writeHandler(context.Background(), []string{"deep/nested/dir/file.txt", "content"}, "")
	if err != nil {
		t.Fatalf("writeHandler error: %v", err)
	}
	if !strings.Contains(output, "wrote") {
		t.Errorf("writeHandler = %q, want wrote confirmation", output)
	}

	data, err := os.ReadFile(filepath.Join(dir, "deep", "nested", "dir", "file.txt"))
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("file content = %q, want %q", string(data), "content")
	}
}

func TestWriteNoArgs(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.writeHandler(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("writeHandler error: %v", err)
	}
	if output != "usage: write <path> [content]" {
		t.Errorf("writeHandler no args = %q, want usage", output)
	}
}

func TestSeeImageFile(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	// Create a fake PNG file (with PNG header bytes).
	path := filepath.Join(dir, "image.png")
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if err := os.WriteFile(path, append(pngHeader, make([]byte, 100)...), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.seeHandler(context.Background(), []string{"image.png"}, "")
	if err != nil {
		t.Fatalf("seeHandler error: %v", err)
	}
	if !strings.Contains(output, "image.png") {
		t.Errorf("seeHandler = %q, want image metadata", output)
	}
	if !strings.Contains(output, "png") {
		t.Errorf("seeHandler = %q, want format info", output)
	}
}

func TestSeeNotImage(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	// Create a text file.
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.seeHandler(context.Background(), []string{"file.txt"}, "")
	if err != nil {
		t.Fatalf("seeHandler error: %v", err)
	}
	if !strings.Contains(output, "[error]") || !strings.Contains(output, "not an image file") {
		t.Errorf("seeHandler = %q, want error about not being an image", output)
	}
	if !strings.Contains(output, "cat") {
		t.Errorf("seeHandler = %q, want suggestion to use cat", output)
	}
}

func TestSeeNoArgs(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.seeHandler(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("seeHandler error: %v", err)
	}
	if output != "usage: see <image-path>" {
		t.Errorf("seeHandler no args = %q, want usage", output)
	}
}

func TestGrepFilter(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	path := filepath.Join(dir, "test.txt")
	content := "hello world\nfoo bar\nhello again\ndone"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.grepHandler(context.Background(), []string{"hello", "test.txt"}, "")
	if err != nil {
		t.Fatalf("grepHandler error: %v", err)
	}
	if !strings.Contains(output, "hello world") {
		t.Errorf("grepHandler = %q, want line containing 'hello world'", output)
	}
	if !strings.Contains(output, "hello again") {
		t.Errorf("grepHandler = %q, want line containing 'hello again'", output)
	}
	if strings.Contains(output, "foo bar") {
		t.Errorf("grepHandler = %q, should not contain 'foo bar'", output)
	}
}

func TestGrepCaseInsensitive(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	path := filepath.Join(dir, "test.txt")
	content := "Hello World\nfoo bar"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.grepHandler(context.Background(), []string{"-i", "hello", "test.txt"}, "")
	if err != nil {
		t.Fatalf("grepHandler error: %v", err)
	}
	if !strings.Contains(output, "Hello World") {
		t.Errorf("grepHandler -i = %q, want 'Hello World' (case insensitive)", output)
	}
}

func TestGrepStdin(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)

	stdin := "line one\nmatch here\nline three"
	output, err := r.grepHandler(context.Background(), []string{"match"}, stdin)
	if err != nil {
		t.Fatalf("grepHandler error: %v", err)
	}
	if !strings.Contains(output, "match here") {
		t.Errorf("grepHandler stdin = %q, want 'match here'", output)
	}
}

func TestGrepInvert(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	path := filepath.Join(dir, "test.txt")
	content := "hello world\nfoo bar\nhello again"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.grepHandler(context.Background(), []string{"-v", "hello", "test.txt"}, "")
	if err != nil {
		t.Fatalf("grepHandler error: %v", err)
	}
	if strings.Contains(output, "hello") {
		t.Errorf("grepHandler -v = %q, should not contain 'hello'", output)
	}
	if !strings.Contains(output, "foo bar") {
		t.Errorf("grepHandler -v = %q, should contain 'foo bar'", output)
	}
}

func TestGrepCount(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	path := filepath.Join(dir, "test.txt")
	content := "hello world\nfoo bar\nhello again"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.grepHandler(context.Background(), []string{"-c", "hello", "test.txt"}, "")
	if err != nil {
		t.Fatalf("grepHandler error: %v", err)
	}
	if output != "2" {
		t.Errorf("grepHandler -c = %q, want '2'", output)
	}
}

func TestGrepNoArgs(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.grepHandler(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("grepHandler error: %v", err)
	}
	if output != "usage: grep <pattern> [-i] [-v] [-c] [file]" {
		t.Errorf("grepHandler no args = %q, want usage", output)
	}
}

func TestStatFile(t *testing.T) {
	dir := setupFS(t)
	r := NewRegistry(presenter.Options{}, nil, nil)

	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := r.statHandler(context.Background(), []string{"test.txt"}, "")
	if err != nil {
		t.Fatalf("statHandler error: %v", err)
	}
	if !strings.Contains(output, "test.txt") {
		t.Errorf("statHandler = %q, want path in output", output)
	}
	if !strings.Contains(output, "file") {
		t.Errorf("statHandler = %q, want 'file' type", output)
	}
}

func TestStatNoArgs(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.statHandler(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("statHandler error: %v", err)
	}
	if output != "usage: stat <path>" {
		t.Errorf("statHandler no args = %q, want usage", output)
	}
}