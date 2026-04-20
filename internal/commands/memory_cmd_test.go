package commands

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/chronocrystal/chronocrystal-go/internal/presenter"
)

// mockMemoryStore implements MemoryStore for testing.
type mockMemoryStore struct {
	facts    []Fact
	summaries []Summary
	nextID   int
}

func (m *mockMemoryStore) SearchFacts(query string, limit int) ([]Fact, error) {
	var results []Fact
	for _, f := range m.facts {
		if strings.Contains(strings.ToLower(f.Content), strings.ToLower(query)) {
			results = append(results, f)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *mockMemoryStore) RecentSummaries(n int) ([]Summary, error) {
	if n > len(m.summaries) {
		n = len(m.summaries)
	}
	return m.summaries[:n], nil
}

func (m *mockMemoryStore) StoreFact(note string) (string, error) {
	m.nextID++
	id := fmt.Sprintf("fact-%d", m.nextID)
	m.facts = append(m.facts, Fact{ID: id, Content: note})
	return id, nil
}

func (m *mockMemoryStore) ListFacts() ([]Fact, error) {
	return m.facts, nil
}

func (m *mockMemoryStore) ForgetFact(id string) error {
	for i, f := range m.facts {
		if f.ID == id {
			m.facts = append(m.facts[:i], m.facts[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("fact %s not found", id)
}

func TestMemoryNoSubcommand(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, &mockMemoryStore{})
	output, err := r.memoryHandler(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if output != "usage: memory search|recent|store|facts|forget" {
		t.Errorf("memoryHandler no args = %q, want usage", output)
	}
}

func TestMemoryNilStore(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.memoryHandler(context.Background(), []string{"search", "test"}, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if output != "[error] memory not available" {
		t.Errorf("memoryHandler nil store = %q, want not available error", output)
	}
}

func TestMemorySearch(t *testing.T) {
	store := &mockMemoryStore{
		facts: []Fact{
			{ID: "1", Content: "Go uses goroutines"},
			{ID: "2", Content: "Python uses asyncio"},
		},
	}
	r := NewRegistry(presenter.Options{}, nil, store)
	output, err := r.memoryHandler(context.Background(), []string{"search", "go"}, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if !strings.Contains(output, "Go uses goroutines") {
		t.Errorf("memoryHandler search = %q, want fact about goroutines", output)
	}
}

func TestMemorySearchNoArgs(t *testing.T) {
	store := &mockMemoryStore{}
	r := NewRegistry(presenter.Options{}, nil, store)
	output, err := r.memoryHandler(context.Background(), []string{"search"}, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if output != "usage: memory search <query>" {
		t.Errorf("memoryHandler search no args = %q, want usage", output)
	}
}

func TestMemoryRecent(t *testing.T) {
	store := &mockMemoryStore{
		summaries: []Summary{
			{Content: "summary one"},
			{Content: "summary two"},
		},
	}
	r := NewRegistry(presenter.Options{}, nil, store)
	output, err := r.memoryHandler(context.Background(), []string{"recent"}, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if !strings.Contains(output, "summary one") {
		t.Errorf("memoryHandler recent = %q, want summary content", output)
	}
}

func TestMemoryStore(t *testing.T) {
	store := &mockMemoryStore{}
	r := NewRegistry(presenter.Options{}, nil, store)
	output, err := r.memoryHandler(context.Background(), []string{"store", "learned", "something"}, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if !strings.Contains(output, "stored fact") {
		t.Errorf("memoryHandler store = %q, want stored fact confirmation", output)
	}
	if len(store.facts) != 1 {
		t.Errorf("expected 1 fact stored, got %d", len(store.facts))
	}
}

func TestMemoryFacts(t *testing.T) {
	store := &mockMemoryStore{
		facts: []Fact{
			{ID: "1", Content: "fact one"},
			{ID: "2", Content: "fact two"},
		},
	}
	r := NewRegistry(presenter.Options{}, nil, store)
	output, err := r.memoryHandler(context.Background(), []string{"facts"}, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if !strings.Contains(output, "fact one") || !strings.Contains(output, "fact two") {
		t.Errorf("memoryHandler facts = %q, want all facts listed", output)
	}
}

func TestMemoryForget(t *testing.T) {
	store := &mockMemoryStore{
		facts: []Fact{
			{ID: "fact-1", Content: "to delete"},
			{ID: "fact-2", Content: "to keep"},
		},
	}
	r := NewRegistry(presenter.Options{}, nil, store)
	output, err := r.memoryHandler(context.Background(), []string{"forget", "fact-1"}, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if !strings.Contains(output, "forgot fact [fact-1]") {
		t.Errorf("memoryHandler forget = %q, want forgot confirmation", output)
	}
	if len(store.facts) != 1 {
		t.Errorf("expected 1 fact remaining, got %d", len(store.facts))
	}
}

func TestMemoryForgetNoArgs(t *testing.T) {
	store := &mockMemoryStore{}
	r := NewRegistry(presenter.Options{}, nil, store)
	output, err := r.memoryHandler(context.Background(), []string{"forget"}, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if output != "usage: memory forget <id>" {
		t.Errorf("memoryHandler forget no args = %q, want usage", output)
	}
}

func TestMemoryUnknownSubcommand(t *testing.T) {
	store := &mockMemoryStore{}
	r := NewRegistry(presenter.Options{}, nil, store)
	output, err := r.memoryHandler(context.Background(), []string{"unknown"}, "")
	if err != nil {
		t.Fatalf("memoryHandler error: %v", err)
	}
	if !strings.Contains(output, "[error]") || !strings.Contains(output, "unknown memory subcommand") {
		t.Errorf("memoryHandler unknown = %q, want error about unknown subcommand", output)
	}
}