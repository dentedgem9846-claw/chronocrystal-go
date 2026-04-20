package chain

type Operator int

const (
	OpNone Operator = iota
	OpAnd           // &&
	OpOr            // ||
	OpSeq           // ;
	OpPipe          // | (but not ||)
)

func (op Operator) String() string {
	switch op {
	case OpNone:
		return "None"
	case OpAnd:
		return "And"
	case OpOr:
		return "Or"
	case OpSeq:
		return "Seq"
	case OpPipe:
		return "Pipe"
	default:
		return "Unknown"
	}
}

type Segment struct {
	Raw string
	Op  Operator
}

// Parse splits input on chain operators (&&, ||, ;, |) while respecting
// quoted strings. Operators inside single or double quotes are treated as
// literal characters. Returns an empty slice for empty or whitespace-only
// input.
func Parse(input string) []Segment {
	if len(input) == 0 {
		return nil
	}

	var segments []Segment
	var buf []rune
	quote := rune(0) // 0 = not in quotes; otherwise the quote char (' or ")

	runes := []rune(input)
	i := 0

	for i < len(runes) {
		r := runes[i]

		// Track quote state
		if quote != 0 {
			if r == quote {
				buf = append(buf, r)
				quote = 0
			} else {
				buf = append(buf, r)
			}
			i++
			continue
		}

		// Entering quotes
		if r == '\'' || r == '"' {
			quote = r
			buf = append(buf, r)
			i++
			continue
		}

		// Check for two-char operators first (&&, ||) then single-char (;, |)
		switch {
		case r == '&' && i+1 < len(runes) && runes[i+1] == '&':
			segments = append(segments, Segment{Raw: string(buf), Op: OpAnd})
			buf = buf[:0]
			i += 2
		case r == '|' && i+1 < len(runes) && runes[i+1] == '|':
			segments = append(segments, Segment{Raw: string(buf), Op: OpOr})
			buf = buf[:0]
			i += 2
		case r == ';':
			segments = append(segments, Segment{Raw: string(buf), Op: OpSeq})
			buf = buf[:0]
			i++
		case r == '|' && !(i+1 < len(runes) && runes[i+1] == '|'):
			segments = append(segments, Segment{Raw: string(buf), Op: OpPipe})
			buf = buf[:0]
			i++
		default:
			buf = append(buf, r)
			i++
		}
	}

	// Push remaining text as last segment
	remaining := string(buf)
	if len(segments) == 0 {
		// No operators found — single segment if non-empty after trim
		remaining = trimSpace(remaining)
		if remaining == "" {
			return nil
		}
		return []Segment{{Raw: remaining, Op: OpNone}}
	}

	segments = append(segments, Segment{Raw: remaining, Op: OpNone})

	// Trim whitespace from all segment Raw fields
	for j := range segments {
		segments[j].Raw = trimSpace(segments[j].Raw)
	}

	// Filter out segments that are empty after trimming (e.g. trailing "; ")
	// but only if they are not the sole segment.
	filtered := segments[:0]
	for _, s := range segments {
		if s.Raw != "" || s.Op == OpNone {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	return filtered
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && isSpace(rune(s[start])) {
		start++
	}
	for end > start && isSpace(rune(s[end-1])) {
		end--
	}
	return s[start:end]
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}