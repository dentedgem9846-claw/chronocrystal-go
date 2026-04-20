package chain

import (
	"testing"
)

func TestParseEmpty(t *testing.T) {
	got := Parse("")
	if len(got) != 0 {
		t.Errorf("Parse('') = %v, want empty slice", got)
	}
}

func TestParseWhitespaceOnly(t *testing.T) {
	got := Parse("   ")
	if len(got) != 0 {
		t.Errorf("Parse('   ') = %v, want empty slice", got)
	}
}

func TestParseSingle(t *testing.T) {
	got := Parse("ls -la")
	if len(got) != 1 {
		t.Fatalf("Parse('ls -la') returned %d segments, want 1", len(got))
	}
	if got[0].Raw != "ls -la" {
		t.Errorf("Raw = %q, want %q", got[0].Raw, "ls -la")
	}
	if got[0].Op != OpNone {
		t.Errorf("Op = %v, want OpNone", got[0].Op)
	}
}

func TestParsePipe(t *testing.T) {
	got := Parse("cat file | grep error")
	if len(got) != 2 {
		t.Fatalf("returned %d segments, want 2", len(got))
	}
	if got[0].Raw != "cat file" {
		t.Errorf("seg[0].Raw = %q, want %q", got[0].Raw, "cat file")
	}
	if got[0].Op != OpPipe {
		t.Errorf("seg[0].Op = %v, want OpPipe", got[0].Op)
	}
	if got[1].Raw != "grep error" {
		t.Errorf("seg[1].Raw = %q, want %q", got[1].Raw, "grep error")
	}
	if got[1].Op != OpNone {
		t.Errorf("seg[1].Op = %v, want OpNone", got[1].Op)
	}
}

func TestParseAnd(t *testing.T) {
	got := Parse("ls && cat file")
	if len(got) != 2 {
		t.Fatalf("returned %d segments, want 2", len(got))
	}
	if got[0].Raw != "ls" {
		t.Errorf("seg[0].Raw = %q, want %q", got[0].Raw, "ls")
	}
	if got[0].Op != OpAnd {
		t.Errorf("seg[0].Op = %v, want OpAnd", got[0].Op)
	}
	if got[1].Raw != "cat file" {
		t.Errorf("seg[1].Raw = %q, want %q", got[1].Raw, "cat file")
	}
}

func TestParseOr(t *testing.T) {
	got := Parse("cat file || echo not found")
	if len(got) != 2 {
		t.Fatalf("returned %d segments, want 2", len(got))
	}
	if got[0].Raw != "cat file" {
		t.Errorf("seg[0].Raw = %q, want %q", got[0].Raw, "cat file")
	}
	if got[0].Op != OpOr {
		t.Errorf("seg[0].Op = %v, want OpOr", got[0].Op)
	}
	if got[1].Raw != "echo not found" {
		t.Errorf("seg[1].Raw = %q, want %q", got[1].Raw, "echo not found")
	}
}

func TestParseSeq(t *testing.T) {
	got := Parse("echo a ; echo b")
	if len(got) != 2 {
		t.Fatalf("returned %d segments, want 2", len(got))
	}
	if got[0].Raw != "echo a" {
		t.Errorf("seg[0].Raw = %q, want %q", got[0].Raw, "echo a")
	}
	if got[0].Op != OpSeq {
		t.Errorf("seg[0].Op = %v, want OpSeq", got[0].Op)
	}
	if got[1].Raw != "echo b" {
		t.Errorf("seg[1].Raw = %q, want %q", got[1].Raw, "echo b")
	}
}

func TestParseChain(t *testing.T) {
	got := Parse("cat file | grep error | head 10")
	if len(got) != 3 {
		t.Fatalf("returned %d segments, want 3", len(got))
	}
	if got[0].Op != OpPipe || got[0].Raw != "cat file" {
		t.Errorf("seg[0] = {%q, %v}, want {\"cat file\", OpPipe}", got[0].Raw, got[0].Op)
	}
	if got[1].Op != OpPipe || got[1].Raw != "grep error" {
		t.Errorf("seg[1] = {%q, %v}, want {\"grep error\", OpPipe}", got[1].Raw, got[1].Op)
	}
	if got[2].Op != OpNone || got[2].Raw != "head 10" {
		t.Errorf("seg[2] = {%q, %v}, want {\"head 10\", OpNone}", got[2].Raw, got[2].Op)
	}
}

func TestParseMixed(t *testing.T) {
	got := Parse("ls && cat file || echo missing ; done")
	if len(got) != 4 {
		t.Fatalf("returned %d segments, want 4", len(got))
	}
	want := []Segment{
		{Raw: "ls", Op: OpAnd},
		{Raw: "cat file", Op: OpOr},
		{Raw: "echo missing", Op: OpSeq},
		{Raw: "done", Op: OpNone},
	}
	for i, w := range want {
		if got[i].Raw != w.Raw {
			t.Errorf("seg[%d].Raw = %q, want %q", i, got[i].Raw, w.Raw)
		}
		if got[i].Op != w.Op {
			t.Errorf("seg[%d].Op = %v, want %v", i, got[i].Op, w.Op)
		}
	}
}

func TestParseQuotedPipe(t *testing.T) {
	got := Parse("echo 'a|b' | grep x")
	if len(got) != 2 {
		t.Fatalf("returned %d segments, want 2", len(got))
	}
	if got[0].Raw != "echo 'a|b'" {
		t.Errorf("seg[0].Raw = %q, want %q", got[0].Raw, "echo 'a|b'")
	}
	if got[0].Op != OpPipe {
		t.Errorf("seg[0].Op = %v, want OpPipe", got[0].Op)
	}
	if got[1].Raw != "grep x" {
		t.Errorf("seg[1].Raw = %q, want %q", got[1].Raw, "grep x")
	}
}

func TestParseQuotedAnd(t *testing.T) {
	got := Parse("echo 'a&&b' && ls")
	if len(got) != 2 {
		t.Fatalf("returned %d segments, want 2", len(got))
	}
	if got[0].Raw != "echo 'a&&b'" {
		t.Errorf("seg[0].Raw = %q, want %q", got[0].Raw, "echo 'a&&b'")
	}
	if got[0].Op != OpAnd {
		t.Errorf("seg[0].Op = %v, want OpAnd", got[0].Op)
	}
	if got[1].Raw != "ls" {
		t.Errorf("seg[1].Raw = %q, want %q", got[1].Raw, "ls")
	}
}

func TestParseQuotedDouble(t *testing.T) {
	got := Parse(`echo "a;b" ; ls`)
	if len(got) != 2 {
		t.Fatalf("returned %d segments, want 2", len(got))
	}
	if got[0].Raw != `echo "a;b"` {
		t.Errorf("seg[0].Raw = %q, want %q", got[0].Raw, `echo "a;b"`)
	}
	if got[0].Op != OpSeq {
		t.Errorf("seg[0].Op = %v, want OpSeq", got[0].Op)
	}
	if got[1].Raw != "ls" {
		t.Errorf("seg[1].Raw = %q, want %q", got[1].Raw, "ls")
	}
}

func TestParseWhitespace(t *testing.T) {
	got := Parse("  ls  |  grep x  ")
	if len(got) != 2 {
		t.Fatalf("returned %d segments, want 2", len(got))
	}
	if got[0].Raw != "ls" {
		t.Errorf("seg[0].Raw = %q, want %q", got[0].Raw, "ls")
	}
	if got[0].Op != OpPipe {
		t.Errorf("seg[0].Op = %v, want OpPipe", got[0].Op)
	}
	if got[1].Raw != "grep x" {
		t.Errorf("seg[1].Raw = %q, want %q", got[1].Raw, "grep x")
	}
}

func TestParseOrNotPipe(t *testing.T) {
	got := Parse("ls || cat")
	if len(got) != 2 {
		t.Fatalf("returned %d segments, want 2", len(got))
	}
	if got[0].Op != OpOr {
		t.Errorf("seg[0].Op = %v, want OpOr (not two OpPipe)", got[0].Op)
	}
	if got[0].Raw != "ls" {
		t.Errorf("seg[0].Raw = %q, want %q", got[0].Raw, "ls")
	}
	if got[1].Raw != "cat" {
		t.Errorf("seg[1].Raw = %q, want %q", got[1].Raw, "cat")
	}
}

func TestOperatorString(t *testing.T) {
	cases := map[Operator]string{
		OpNone: "None",
		OpAnd:  "And",
		OpOr:   "Or",
		OpSeq:  "Seq",
		OpPipe: "Pipe",
	}
	for op, want := range cases {
		got := op.String()
		if got != want {
			t.Errorf("Operator(%d).String() = %q, want %q", op, got, want)
		}
	}
}