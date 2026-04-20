package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// helpHandler lists all available commands with one-line descriptions.
func (r *Registry) helpHandler(_ context.Context, _ []string, _ string) (string, error) {
	names := make([]string, 0, len(r.descriptions))
	for name := range r.descriptions {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		b.WriteString(fmt.Sprintf("  %s — %s\n", name, r.descriptions[name]))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}