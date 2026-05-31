package twitch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/kudryavtsevmakar/valovoting/internal/config"
)

const maxPollChoices = 5

type PollAPI struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewPollAPI(cfg *config.Config) *PollAPI {
	return &PollAPI{cfg: cfg, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

type createPollReq struct {
	BroadcasterID string      `json:"broadcaster_id"`
	Title         string      `json:"title"`
	Choices       []apiChoice `json:"choices"`
	Duration      int         `json:"duration"`
}

type apiChoice struct {
	Title string `json:"title"`
}

// CreateMapPoll builds a Twitch Poll from the current competitive map pool,
// removing any maps listed in excludeNames (case-insensitive prefix match).
// Twitch caps choices at 5; if more remain they are randomly sampled.
func (a *PollAPI) CreateMapPoll(excludeNames []string, duration int) error {
	excluded := map[string]bool{}
	for _, name := range excludeNames {
		if m := matchMap(name); m != "" {
			excluded[m] = true
		} else {
			log.Printf("pollapi: unknown map %q — ignoring", name)
		}
	}

	candidates := make([]string, 0, len(CompetitiveMaps))
	for _, m := range CompetitiveMaps {
		if !excluded[m] {
			candidates = append(candidates, m)
		}
	}

	if len(candidates) < 2 {
		return fmt.Errorf("only %d map(s) left after exclusions — need at least 2", len(candidates))
	}

	if len(candidates) > maxPollChoices {
		log.Printf("pollapi: %d maps remain, randomly sampling %d", len(candidates), maxPollChoices)
		rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
		candidates = candidates[:maxPollChoices]
	}

	choices := make([]apiChoice, len(candidates))
	for i, c := range candidates {
		choices[i] = apiChoice{Title: c}
	}

	body := createPollReq{
		BroadcasterID: a.cfg.TwitchBroadcasterID,
		Title:         "Выбор карты",
		Choices:       choices,
		Duration:      duration,
	}
	raw, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "https://api.twitch.tv/helix/polls", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.cfg.TwitchAccessToken)
	req.Header.Set("Client-Id", a.cfg.TwitchClientID)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twitch API %d: %s", resp.StatusCode, b)
	}

	log.Printf("pollapi: poll created — maps: %v, duration: %ds", candidates, duration)
	return nil
}
