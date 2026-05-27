package poll

import "time"

type Choice struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Votes int    `json:"votes"`
}

type State struct {
	Phase           string    `json:"phase"`
	PollID          string    `json:"poll_id"`
	Title           string    `json:"title"`
	Status          string    `json:"status"`
	Choices         []Choice  `json:"choices"`
	DurationSeconds int       `json:"duration_seconds"`
	StartedAt       time.Time `json:"started_at"`
	EndsAt          time.Time `json:"ends_at"`
	ServerNow       time.Time `json:"server_time"`
	Winner          *Choice   `json:"winner"`
}

func IdleState() State {
	return State{Phase: "idle", ServerNow: time.Now()}
}
