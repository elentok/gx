package wire

// EventSessionChatChanged announces that a working session's transcript grew
// or shrank; its address is the session id.
const EventSessionChatChanged = "session-chat-changed"

// ReasonSessionNotWorking refuses an interrupt of a session with no turn to end.
const ReasonSessionNotWorking = "session-not-working"
