package channel

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Classification of incoming messages.
type Classification string

const (
	ClassChat  Classification = "chat"
	ClassOrder Classification = "order"
	ClassStop  Classification = "stop"
)

// SimpleX event types.
type EventType string

const (
	EventNewChatItems    EventType = "newChatItems"
	EventContactConnected EventType = "contactConnected"
	EventContactSndReady  EventType = "contactSndReady"
)

// Event represents a parsed SimpleX Chat event.
type Event struct {
	Type EventType
	// NewChatItems fields
	ChatItems []ChatItem
	// ContactConnected fields
	Contact Contact
}

// ChatItem represents a message item from SimpleX.
type ChatItem struct {
	ChatInfo ChatInfo
	Content  MsgContent
}

// ChatInfo identifies the conversation.
type ChatInfo struct {
	// Direct chat
	ContactID   string
	DisplayName string
	// Group chat (future)
	GroupID string
}

// MsgContent holds the message text.
type MsgContent struct {
	Text string
}

// Contact represents a connected SimpleX user.
type Contact struct {
	ContactID   string
	DisplayName string
}

// ChatRef identifies a target for sending messages.
type ChatRef struct {
	ContactID string
	GroupID   string // empty for direct chat
}

// ComposedMessage is a message to send via SimpleX.
type ComposedMessage struct {
	Text string `json:"text"`
}

// --- SimpleX protocol command builders ---

// BuildSendCommand constructs the /_send command.
func BuildSendCommand(chatRef ChatRef, text string) string {
	composed := []ComposedMessage{{Text: text}}
	composedJSON, _ := json.Marshal(composed)
	ref := chatRef.ContactID
	if chatRef.GroupID != "" {
		ref = chatRef.GroupID
	}
	return fmt.Sprintf("/_send %s json %s", ref, string(composedJSON))
}

// BuildCreateAddressCommand constructs the /_address command.
func BuildCreateAddressCommand(userID int64) string {
	return fmt.Sprintf("/_address %d", userID)
}

// BuildSetAddressSettingsCommand constructs the /_address_settings command for auto-accept.
func BuildSetAddressSettingsCommand(userID int64, autoAccept bool) string {
	settings := map[string]interface{}{
		"autoAccept": map[string]interface{}{
			"auto": autoAccept,
		},
	}
	settingsJSON, _ := json.Marshal(settings)
	return fmt.Sprintf("/_address_settings %d %s", userID, string(settingsJSON))
}

// BuildListContactsCommand constructs the /_list command.
func BuildListContactsCommand(userID int64) string {
	return fmt.Sprintf("/_list %d", userID)
}

// --- SimpleX protocol response parsers ---

// ParseResponse parses a raw JSON line from SimpleX Chat.
func ParseResponse(line string) (interface{}, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse SimpleX response: %w", err)
	}

	typeField, ok := raw["type"]
	if !ok {
		return nil, fmt.Errorf("missing type field in SimpleX response")
	}

	var eventType string
	if err := json.Unmarshal(typeField, &eventType); err != nil {
		return nil, fmt.Errorf("failed to parse type field: %w", err)
	}

	switch EventType(eventType) {
	case EventNewChatItems:
		var resp NewChatItemsResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return nil, err
		}
		return resp, nil
	case EventContactConnected:
		var resp ContactConnectedResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return nil, err
		}
		return resp, nil
	case EventContactSndReady:
		var resp ContactSndReadyResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return nil, err
		}
		return resp, nil
	default:
		// Unknown event type, return raw
		return raw, nil
	}
}

// --- SimpleX response types ---

type NewChatItemsResponse struct {
	Type     string     `json:"type"`
	User     User       `json:"user"`
	ChatItems []RawChatItem `json:"chatItems"`
}

type ContactConnectedResponse struct {
	Type    string  `json:"type"`
	User    User    `json:"user"`
	Contact Contact `json:"contact"`
}

type ContactSndReadyResponse struct {
	Type    string  `json:"type"`
	User    User    `json:"user"`
	Contact Contact `json:"contact"`
}

type User struct {
	UserID int64 `json:"userId"`
}

type RawChatItem struct {
	ChatInfo RawChatInfo `json:"chatInfo"`
	ChatItem RawMsgItem  `json:"chatItem"`
}

type RawChatInfo struct {
	// Direct chat
	ContactID   string `json:"contactId"`
	DisplayName string `json:"displayName"`
	// Could also be groupInfo, handled later
}

type RawMsgItem struct {
	ID        int64  `json:"itemId"`
	Content   MsgContent `json:"content"`
	CreatedAt string `json:"createdAt"`
}

// --- Utility ---

// FormatTimestamp parses SimpleX timestamp formats.
func FormatTimestamp(ts string) time.Time {
	t, _ := time.Parse(time.RFC3339, ts)
	return t
}

// ExtractContactID extracts a contact ID from a raw chat info.
func ExtractContactID(info RawChatInfo) string {
	if info.ContactID != "" {
		return info.ContactID
	}
	return ""
}

// ParseInt safely parses a string to int64.
func ParseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}