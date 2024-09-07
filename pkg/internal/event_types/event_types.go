package event_types

const (
	ALL           = "ALL"
	MESSAGE       = "MESSAGE"
	READ_RECEIPT  = "READ_RECEIPT"
	PRESENCE      = "PRESENCE"
	HISTORY_SYNC  = "HISTORY_SYNC"
	CHAT_PRESENCE = "CHAT_PRESENCE"
	CALL          = "CALL"
	CONNECTION    = "CONNECTION"
	LABEL         = "LABEL"
	CONTACT       = "CONTACT"
	GROUP         = "GROUP"
	NEWSLETTER    = "NEWSLETTER"
)

var validEventTypes = map[string]bool{
	ALL:           true,
	MESSAGE:       true,
	READ_RECEIPT:  true,
	PRESENCE:      true,
	HISTORY_SYNC:  true,
	CHAT_PRESENCE: true,
	CALL:          true,
	CONNECTION:    true,
	LABEL:         true,
	CONTACT:       true,
	GROUP:         true,
	NEWSLETTER:    true,
}

func IsEventType(eventType string) bool {
	return validEventTypes[eventType]
}
