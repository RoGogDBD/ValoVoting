package twitch

import "time"

// EventSub WebSocket envelope

type wsMessage struct {
	Metadata wsMetadata `json:"metadata"`
	Payload  wsPayload  `json:"payload"`
}

type wsMetadata struct {
	MessageID        string    `json:"message_id"`
	MessageType      string    `json:"message_type"`
	MessageTimestamp time.Time `json:"message_timestamp"`
	SubscriptionType string    `json:"subscription_type"`
}

type wsPayload struct {
	Session      *wsSession      `json:"session,omitempty"`
	Subscription *wsSubscription `json:"subscription,omitempty"`
	Event        *PollEvent      `json:"event,omitempty"`
}

type wsSession struct {
	ID                      string `json:"id"`
	Status                  string `json:"status"`
	KeepaliveTimeoutSeconds int    `json:"keepalive_timeout_seconds"`
	ReconnectURL            string `json:"reconnect_url"`
}

type wsSubscription struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

// Poll event payload

type PollEvent struct {
	ID                  string       `json:"id"`
	BroadcasterUserID   string       `json:"broadcaster_user_id"`
	Title               string       `json:"title"`
	Choices             []PollChoice `json:"choices"`
	Status              string       `json:"status"`
	StartedAt           time.Time    `json:"started_at"`
	EndsAt              time.Time    `json:"ends_at"`
	BitsVotingEnabled   bool         `json:"bits_voting_enabled"`
	ChannelPointsVoting interface{}  `json:"channel_points_voting"`
}

type PollChoice struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	BitsVotes          int    `json:"bits_votes"`
	ChannelPointsVotes int    `json:"channel_points_votes"`
	Votes              int    `json:"votes"`
}

// Subscription create request/response

type subRequest struct {
	Type      string       `json:"type"`
	Version   string       `json:"version"`
	Condition subCondition `json:"condition"`
	Transport subTransport `json:"transport"`
}

type subCondition struct {
	BroadcasterUserID string `json:"broadcaster_user_id"`
}

type subTransport struct {
	Method    string `json:"method"`
	SessionID string `json:"session_id"`
}
