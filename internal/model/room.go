package model

import (
	"time"
)

type RoomStatus string

const (
	StatusWaiting       RoomStatus = "waiting"
	StatusPlaying       RoomStatus = "playing"
	StatusBetweenRounds RoomStatus = "between_rounds"
	StatusFinished      RoomStatus = "finished"
)

type WordsSource string

const (
	WordsStandard WordsSource = "standard"
	WordsCustom   WordsSource = "custom"
)

type WinCondition string

const (
	WinByPoints WinCondition = "points"
	WinByRounds WinCondition = "rounds"
)

type RoomSettings struct {
	NumTeams       int          `json:"num_teams"`
	TeamNames      []string     `json:"team_names"`
	WordsSource    WordsSource  `json:"words_source"`
	CustomWords    []string     `json:"custom_words,omitempty"`
	Difficulty     string       `json:"difficulty"`
	RoundDuration  int          `json:"round_duration"`
	WinCondition   WinCondition `json:"win_condition"`
	WinValue       int          `json:"win_value"`
	PenaltyForSkip bool         `json:"penalty_for_skip"`
	Language       string       `json:"language"`
	BettingEnabled bool         `json:"betting_enabled"`
	TasksEnabled   bool         `json:"tasks_enabled"`
	Tasks          []string     `json:"tasks,omitempty"`
}

type Team struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Score int    `json:"score"`
}

type Round struct {
	Number           int       `json:"number"`
	TeamID           string    `json:"team_id"`
	Words            []string  `json:"words"`
	CurrentWordIndex int       `json:"current_word_index"`
	Correct          []string  `json:"correct"`
	Skipped          []string  `json:"skipped"`
	StartedAt        time.Time `json:"started_at"`
	Duration         int       `json:"duration"`
	Finished         bool      `json:"finished"`
	Task             string    `json:"task,omitempty"`
}

// CurrentWord returns the current word for the explainer.
func (r *Round) CurrentWord() string {
	if r.CurrentWordIndex < len(r.Words) {
		return r.Words[r.CurrentWordIndex]
	}
	return ""
}

type Room struct {
	ID             string       `json:"id"`
	HostID         string       `json:"host_id"`
	Settings       RoomSettings `json:"settings"`
	Teams          []Team       `json:"teams"`
	Status         RoomStatus   `json:"status"`
	CurrentRound   *Round       `json:"current_round,omitempty"`
	RoundHistory   []Round      `json:"round_history"`
	CurrentTeamIdx int          `json:"current_team_idx"`
	DisputeID      string       `json:"dispute_id,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
}

// NextTeamIdx returns the index of the next team to play.
func (r *Room) NextTeamIdx() int {
	return (r.CurrentTeamIdx + 1) % len(r.Teams)
}

// IsFinished checks if the game win condition is met.
func (r *Room) IsFinished() bool {
	if r.Settings.WinCondition == WinByPoints {
		for _, t := range r.Teams {
			if t.Score >= r.Settings.WinValue {
				return true
			}
		}
	} else if r.Settings.WinCondition == WinByRounds {
		return len(r.RoundHistory) >= r.Settings.WinValue*len(r.Teams)
	}
	return false
}

// Winner returns the team with the highest score.
func (r *Room) Winner() *Team {
	if len(r.Teams) == 0 {
		return nil
	}
	winner := &r.Teams[0]
	for i := range r.Teams {
		if r.Teams[i].Score > winner.Score {
			winner = &r.Teams[i]
		}
	}
	return winner
}
