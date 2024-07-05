package event_types

const (
	MESSAGE       = "MESSAGE"
	READ_RECEIPT  = "READ_RECEIPT"
	PRESENCE      = "PRESENCE"
	HISTORY_SYNC  = "HISTORY_SYNC"
	CHAT_PRESENCE = "CHAT_PRESENCE"
	CALL          = "CALL"
	ALL           = "ALL"
)

var validEventTypes = map[string]bool{
	MESSAGE:       true,
	READ_RECEIPT:  true,
	PRESENCE:      true,
	HISTORY_SYNC:  true,
	CHAT_PRESENCE: true,
	CALL:          true,
	ALL:           true,
}

func IsEventType(eventType string) bool {
	return validEventTypes[eventType]
}
