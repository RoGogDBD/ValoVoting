package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kudryavtsevmakar/valovoting/internal/config"
	"github.com/kudryavtsevmakar/valovoting/internal/poll"
)

const eventsubURL = "wss://eventsub.wss.twitch.tv/ws"

type EventSubClient struct {
	cfg         *config.Config
	pollService *poll.Service
	onUpdate    func(poll.State)
	httpClient  *http.Client
}

func NewEventSubClient(cfg *config.Config, svc *poll.Service, onUpdate func(poll.State)) *EventSubClient {
	return &EventSubClient{
		cfg:         cfg,
		pollService: svc,
		onUpdate:    onUpdate,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Run connects and reconnects forever until ctx is cancelled.
func (c *EventSubClient) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if err := c.connect(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("eventsub: disconnected (%v), reconnecting in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff = time.Duration(math.Min(float64(backoff*2), float64(30*time.Second)))
			}
		} else {
			backoff = time.Second
		}
	}
}

func (c *EventSubClient) connect(ctx context.Context) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, eventsubURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	log.Println("eventsub: connected")

	var sessionID string

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var msg wsMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("eventsub: unmarshal error: %v", err)
			continue
		}

		switch msg.Metadata.MessageType {
		case "session_welcome":
			if msg.Payload.Session == nil {
				continue
			}
			sessionID = msg.Payload.Session.ID
			log.Printf("eventsub: session_welcome id=%s", sessionID)
			if err := c.subscribe(sessionID); err != nil {
				return fmt.Errorf("subscribe: %w", err)
			}

		case "session_keepalive":
			// no-op

		case "session_reconnect":
			if msg.Payload.Session != nil && msg.Payload.Session.ReconnectURL != "" {
				log.Printf("eventsub: reconnect requested to %s", msg.Payload.Session.ReconnectURL)
				// close current conn; outer loop will reconnect
				return nil
			}

		case "notification":
			c.handleNotification(msg)

		case "revocation":
			log.Printf("eventsub: subscription revoked type=%s", msg.Metadata.SubscriptionType)
		}
	}
}

func (c *EventSubClient) subscribe(sessionID string) error {
	types := []string{"channel.poll.begin", "channel.poll.progress", "channel.poll.end"}
	for _, t := range types {
		if err := c.createSub(sessionID, t); err != nil {
			return err
		}
	}
	return nil
}

func (c *EventSubClient) createSub(sessionID, subType string) error {
	body := subRequest{
		Type:    subType,
		Version: "1",
		Condition: subCondition{BroadcasterUserID: c.cfg.TwitchBroadcasterID},
		Transport: subTransport{Method: "websocket", SessionID: sessionID},
	}
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "https://api.twitch.tv/helix/eventsub/subscriptions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.TwitchAccessToken)
	req.Header.Set("Client-Id", c.cfg.TwitchClientID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("createSub %s: %w", subType, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("createSub %s: HTTP %d: %s", subType, resp.StatusCode, raw)
	}
	log.Printf("eventsub: subscribed to %s", subType)
	return nil
}

func (c *EventSubClient) handleNotification(msg wsMessage) {
	if msg.Payload.Event == nil {
		return
	}
	ev := msg.Payload.Event
	subType := msg.Metadata.SubscriptionType
	log.Printf("eventsub: notification type=%s poll=%s", subType, ev.ID)

	state := c.buildState(ev, subType)
	c.pollService.SetState(state)
	c.onUpdate(state)
}

func (c *EventSubClient) buildState(ev *PollEvent, subType string) poll.State {
	choices := make([]poll.Choice, len(ev.Choices))
	for i, ch := range ev.Choices {
		choices[i] = poll.Choice{ID: ch.ID, Title: ch.Title, Votes: ch.Votes}
	}

	status := "active"
	var winner *poll.Choice
	if subType == "channel.poll.end" {
		status = ev.Status // "completed" or "archived"
		winner = pickWinner(choices)
	}

	phase := detectPhase(ev.Title)
	endsAt := ev.EndsAt
	if endsAt.IsZero() && subType == "channel.poll.begin" {
		endsAt = ev.StartedAt.Add(time.Duration(60) * time.Second)
	}

	return poll.State{
		Phase:           phase,
		PollID:          ev.ID,
		Title:           ev.Title,
		Status:          status,
		Choices:         choices,
		DurationSeconds: int(endsAt.Sub(ev.StartedAt).Seconds()),
		StartedAt:       ev.StartedAt,
		EndsAt:          endsAt,
		Winner:          winner,
	}
}

func pickWinner(choices []poll.Choice) *poll.Choice {
	if len(choices) == 0 {
		return nil
	}
	best := choices[0]
	for _, c := range choices[1:] {
		if c.Votes > best.Votes {
			best = c
		}
	}
	return &best
}

func detectPhase(title string) string {
	for _, kw := range []string{"карт", "map", "Map", "MAP"} {
		if contains(title, kw) {
			return "map"
		}
	}
	for _, kw := range []string{"агент", "agent", "Agent", "AGENT"} {
		if contains(title, kw) {
			return "agent"
		}
	}
	return "map"
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
