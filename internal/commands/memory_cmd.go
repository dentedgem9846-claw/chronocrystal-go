package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// memoryHandler dispatches memory subcommands.
func (r *Registry) memoryHandler(ctx context.Context, args []string, _ string) (string, error) {
	if r.memStore == nil {
		return "[error] memory not available", nil
	}

	if len(args) == 0 {
		return "usage: memory search|recent|store|facts|forget", nil
	}

	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "search":
		return r.memorySearch(ctx, subArgs)
	case "recent":
		return r.memoryRecent(subArgs)
	case "store":
		return r.memoryStore(subArgs)
	case "facts":
		return r.memoryFacts()
	case "forget":
		return r.memoryForget(subArgs)
	default:
		return fmt.Sprintf("[error] unknown memory subcommand: %s. Available: search, recent, store, facts, forget", subcmd), nil
	}
}

func (r *Registry) memorySearch(_ context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "usage: memory search <query>", nil
	}
	query := strings.Join(args, " ")
	facts, err := r.memStore.SearchFacts(query, 10)
	if err != nil {
		return "", fmt.Errorf("memory search: %w", err)
	}
	if len(facts) == 0 {
		return "no facts found", nil
	}
	var b strings.Builder
	for _, f := range facts {
		b.WriteString(fmt.Sprintf("  [%s] %s\n", f.ID, f.Content))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (r *Registry) memoryRecent(args []string) (string, error) {
	n := 5
	if len(args) > 0 {
		// Parse n from args.
		var err error
		if _, e := fmt.Sscanf(args[0], "%d", &n); e != nil {
			err = e
		}
		if err != nil || n <= 0 {
			n = 5
		}
	}
	summaries, err := r.memStore.RecentSummaries(n)
	if err != nil {
		return "", fmt.Errorf("memory recent: %w", err)
	}
	if len(summaries) == 0 {
		return "no recent summaries", nil
	}
	var b strings.Builder
	for _, s := range summaries {
		b.WriteString(fmt.Sprintf("  %s\n", s.Content))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (r *Registry) memoryStore(args []string) (string, error) {
	if len(args) == 0 {
		return "usage: memory store <note>", nil
	}
	note := strings.Join(args, " ")
	id, err := r.memStore.StoreFact(note)
	if err != nil {
		return "", fmt.Errorf("memory store: %w", err)
	}
	return fmt.Sprintf("stored fact [%s]", id), nil
}

func (r *Registry) memoryFacts() (string, error) {
	facts, err := r.memStore.ListFacts()
	if err != nil {
		return "", fmt.Errorf("memory facts: %w", err)
	}
	if len(facts) == 0 {
		return "no facts stored", nil
	}

	// Sort by ID for deterministic output.
	sort.Slice(facts, func(i, j int) bool {
		return facts[i].ID < facts[j].ID
	})

	var b strings.Builder
	for _, f := range facts {
		b.WriteString(fmt.Sprintf("  [%s] %s\n", f.ID, f.Content))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (r *Registry) memoryForget(args []string) (string, error) {
	if len(args) == 0 {
		return "usage: memory forget <id>", nil
	}
	id := args[0]
	if err := r.memStore.ForgetFact(id); err != nil {
		return "", fmt.Errorf("memory forget: %w", err)
	}
	return fmt.Sprintf("forgot fact [%s]", id), nil
}