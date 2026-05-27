package twitch

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/kudryavtsevmakar/valovoting/internal/config"
)

const ircAddr = "irc.chat.twitch.tv:6697"

type ChatBot struct {
	cfg     *config.Config
	pollAPI *PollAPI
}

type ircMessage struct {
	tags    map[string]string
	user    string
	channel string
	text    string
}

func NewChatBot(cfg *config.Config, pollAPI *PollAPI) *ChatBot {
	return &ChatBot{cfg: cfg, pollAPI: pollAPI}
}

// Run connects to Twitch IRC and reconnects with exponential backoff until ctx is cancelled.
func (b *ChatBot) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if err := b.connect(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("chat: disconnected (%v), reconnecting in %s", err, backoff)
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

func (b *ChatBot) connect(ctx context.Context) error {
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp", ircAddr,
		&tls.Config{ServerName: "irc.chat.twitch.tv"},
	)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	log.Printf("chat: connected to Twitch IRC, joining #%s", b.cfg.TwitchChannel)

	send := func(line string) { fmt.Fprintf(conn, "%s\r\n", line) }

	// Request badge/tag metadata so we can check mod status
	send("CAP REQ :twitch.tv/tags twitch.tv/commands")
	send("PASS oauth:" + b.cfg.TwitchAccessToken)
	send("NICK " + strings.ToLower(b.cfg.TwitchChannel))
	send("JOIN #" + strings.ToLower(b.cfg.TwitchChannel))

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil
		}
		line := scanner.Text()

		if strings.HasPrefix(line, "PING") {
			send("PONG :tmi.twitch.tv")
			continue
		}

		msg, ok := parseIRCLine(line)
		if !ok {
			continue
		}
		if !strings.EqualFold(msg.channel, b.cfg.TwitchChannel) {
			continue
		}

		b.handleMessage(msg)
	}

	return scanner.Err()
}

func (b *ChatBot) handleMessage(msg *ircMessage) {
	if !isAuthorized(msg, b.cfg.TwitchChannel) {
		return
	}

	// Match command prefix case-insensitively
	text := msg.text
	cmd := b.cfg.ChatCommand
	if !strings.EqualFold(text[:min(len(text), len(cmd))], cmd) {
		return
	}
	if len(text) < len(cmd) {
		return
	}
	rest := text[len(cmd):]
	// Command must be followed by end-of-string or a space
	if len(rest) > 0 && rest[0] != ' ' {
		return
	}

	excludeMaps, duration := parseArgs(strings.TrimSpace(rest), b.cfg.DefaultPollDuration)
	log.Printf("chat: %s triggered map vote — exclude=%v duration=%ds", msg.user, excludeMaps, duration)

	if err := b.pollAPI.CreateMapPoll(excludeMaps, duration); err != nil {
		log.Printf("chat: poll creation failed: %v", err)
	}
}

// isAuthorized returns true if the message author is the broadcaster or a moderator.
func isAuthorized(msg *ircMessage, channelName string) bool {
	if strings.EqualFold(msg.user, channelName) {
		return true
	}
	if msg.tags["mod"] == "1" {
		return true
	}
	badges := msg.tags["badges"]
	return strings.Contains(badges, "broadcaster/1") || strings.Contains(badges, "moderator/1")
}

// parseArgs parses the tail of a command message.
//
// Supported syntax (all combinable):
//
//	-bind              exclude Bind
//	-bind,ascent       exclude Bind and Ascent (comma list after a single dash)
//	-bind, ascent      same — trailing comma tokens without dash are matched as maps
//	90                 set poll duration to 90 seconds (15–1800)
func parseArgs(args string, defaultDuration int) (excludeMaps []string, duration int) {
	duration = defaultDuration
	inExcludeGroup := false // true after we've seen a dash token

	for _, token := range strings.Fields(args) {
		token = strings.Trim(token, ",")
		if token == "" {
			continue
		}

		// Duration: bare integer
		if n, err := strconv.Atoi(token); err == nil {
			if n >= 15 && n <= 1800 {
				duration = n
			}
			inExcludeGroup = false
			continue
		}

		// Explicit exclusion starting with dash
		if strings.HasPrefix(token, "-") {
			inExcludeGroup = true
			for _, part := range strings.Split(token[1:], ",") {
				if p := strings.TrimSpace(part); p != "" {
					excludeMaps = append(excludeMaps, p)
				}
			}
			continue
		}

		// Bare word: treat as excluded map if it follows a dash token OR matches a known map
		if inExcludeGroup || matchMap(token) != "" {
			excludeMaps = append(excludeMaps, token)
		}
	}
	return
}

func parseIRCLine(line string) (*ircMessage, bool) {
	tags := map[string]string{}

	// Parse leading @tags
	if strings.HasPrefix(line, "@") {
		idx := strings.Index(line, " ")
		if idx < 0 {
			return nil, false
		}
		for _, kv := range strings.Split(line[1:idx], ";") {
			if i := strings.IndexByte(kv, '='); i >= 0 {
				tags[kv[:i]] = kv[i+1:]
			}
		}
		line = line[idx+1:]
	}

	// :user!user@user.tmi.twitch.tv PRIVMSG #channel :text
	if !strings.HasPrefix(line, ":") {
		return nil, false
	}
	parts := strings.SplitN(line[1:], " ", 4)
	if len(parts) < 4 || parts[1] != "PRIVMSG" {
		return nil, false
	}

	user := parts[0]
	if i := strings.IndexByte(user, '!'); i >= 0 {
		user = user[:i]
	}

	text := parts[3]
	if strings.HasPrefix(text, ":") {
		text = text[1:]
	}

	return &ircMessage{
		tags:    tags,
		user:    user,
		channel: strings.TrimPrefix(parts[2], "#"),
		text:    text,
	}, true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
