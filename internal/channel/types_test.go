package channel

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseResponseNewChatItems(t *testing.T) {
	raw := `{"type":"newChatItems","user":{"userId":1},"chatItems":[{"chatInfo":{"contactId":"c1","displayName":"Alice"},"chatItem":{"itemId":10,"content":{"text":"hello"},"createdAt":"2024-01-01T00:00:00Z"}}]}`
	got, err := ParseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, ok := got.(NewChatItemsResponse)
	if !ok {
		t.Fatalf("expected NewChatItemsResponse, got %T", got)
	}
	if resp.Type != "newChatItems" {
		t.Errorf("expected type newChatItems, got %q", resp.Type)
	}
	if len(resp.ChatItems) != 1 {
		t.Fatalf("expected 1 chat item, got %d", len(resp.ChatItems))
	}
	if resp.ChatItems[0].ChatInfo.ContactID != "c1" {
		t.Errorf("expected contactId c1, got %q", resp.ChatItems[0].ChatInfo.ContactID)
	}
	if resp.ChatItems[0].ChatItem.Content.Text != "hello" {
		t.Errorf("expected text hello, got %q", resp.ChatItems[0].ChatItem.Content.Text)
	}
}

func TestParseResponseContactConnected(t *testing.T) {
	raw := `{"type":"contactConnected","user":{"userId":1},"contact":{"contactId":"c2","displayName":"Bob"}}`
	got, err := ParseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, ok := got.(ContactConnectedResponse)
	if !ok {
		t.Fatalf("expected ContactConnectedResponse, got %T", got)
	}
	if resp.Contact.ContactID != "c2" {
		t.Errorf("expected contactId c2, got %q", resp.Contact.ContactID)
	}
	if resp.Contact.DisplayName != "Bob" {
		t.Errorf("expected displayName Bob, got %q", resp.Contact.DisplayName)
	}
}

func TestParseResponseContactSndReady(t *testing.T) {
	raw := `{"type":"contactSndReady","user":{"userId":1},"contact":{"contactId":"c3","displayName":"Carol"}}`
	got, err := ParseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, ok := got.(ContactSndReadyResponse)
	if !ok {
		t.Fatalf("expected ContactSndReadyResponse, got %T", got)
	}
	if resp.Contact.ContactID != "c3" {
		t.Errorf("expected contactId c3, got %q", resp.Contact.ContactID)
	}
}

func TestParseResponseUnknownType(t *testing.T) {
	raw := `{"type":"someFutureEvent","data":"value"}`
	got, err := ParseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := got.(map[string]json.RawMessage)
	if !ok {
		t.Fatalf("expected map[string]json.RawMessage, got %T", got)
	}
	if string(m["type"]) != `"someFutureEvent"` {
		t.Errorf("expected type field in raw map, got %q", string(m["type"]))
	}
}

func TestParseResponseInvalidJSON(t *testing.T) {
	_, err := ParseResponse("not json at all")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseResponseEmptyLine(t *testing.T) {
	got, err := ParseResponse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for empty line, got %v", got)
	}

	got, err = ParseResponse("   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for whitespace, got %v", got)
	}
}

func TestBuildSendCommand(t *testing.T) {
	ref := ChatRef{ContactID: "c1"}
	cmd := BuildSendCommand(ref, "hello world")
	expected := `/_send c1 json [{"text":"hello world"}]`
	if cmd != expected {
		t.Errorf("expected %q, got %q", expected, cmd)
	}
}

func TestBuildSendCommandGroupID(t *testing.T) {
	ref := ChatRef{ContactID: "c1", GroupID: "g1"}
	cmd := BuildSendCommand(ref, "hello")
	expected := `/_send g1 json [{"text":"hello"}]`
	if cmd != expected {
		t.Errorf("expected %q, got %q", expected, cmd)
	}
}

func TestBuildCreateAddressCommand(t *testing.T) {
	cmd := BuildCreateAddressCommand(42)
	expected := "/_address 42"
	if cmd != expected {
		t.Errorf("expected %q, got %q", expected, cmd)
	}
}

func TestBuildSetAddressSettingsCommand(t *testing.T) {
	cmd := BuildSetAddressSettingsCommand(42, true)
	expected := `/_address_settings 42 {"autoAccept":{"auto":true}}`
	if cmd != expected {
		t.Errorf("expected %q, got %q", expected, cmd)
	}

	cmdFalse := BuildSetAddressSettingsCommand(7, false)
	expectedFalse := `/_address_settings 7 {"autoAccept":{"auto":false}}`
	if cmdFalse != expectedFalse {
		t.Errorf("expected %q, got %q", expectedFalse, cmdFalse)
	}
}

func TestBuildListContactsCommand(t *testing.T) {
	cmd := BuildListContactsCommand(99)
	expected := "/_list 99"
	if cmd != expected {
		t.Errorf("expected %q, got %q", expected, cmd)
	}
}

func TestFormatTimestamp(t *testing.T) {
	ts := "2024-06-15T10:30:00Z"
	got := FormatTimestamp(ts)
	want := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestFormatTimestampInvalid(t *testing.T) {
	got := FormatTimestamp("not-a-timestamp")
	if !got.IsZero() {
		t.Errorf("expected zero time for invalid input, got %v", got)
	}
}

func TestExtractContactID(t *testing.T) {
	info := RawChatInfo{ContactID: "c1", DisplayName: "Alice"}
	if got := ExtractContactID(info); got != "c1" {
		t.Errorf("expected c1, got %q", got)
	}

	empty := RawChatInfo{DisplayName: "Alice"}
	if got := ExtractContactID(empty); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestParseInt(t *testing.T) {
	if got := ParseInt("42"); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	if got := ParseInt("0"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
	if got := ParseInt("-7"); got != -7 {
		t.Errorf("expected -7, got %d", got)
	}
	if got := ParseInt("abc"); got != 0 {
		t.Errorf("expected 0 for invalid input, got %d", got)
	}
}