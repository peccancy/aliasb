package dto

import (
	"net/http"

	"github.com/go-playground/validator/v10"
)

type CreateRoomDTO struct {
	NumTeams       int      `json:"num_teams" validate:"required,min=2,max=20"`
	TeamNames      []string `json:"team_names" validate:"required"`
	WordsSource    string   `json:"words_source" validate:"required,oneof=standard custom"`
	CustomWords    []string `json:"custom_words,omitempty"`
	Difficulty     string   `json:"difficulty" validate:"omitempty,oneof=easy medium hard"`
	RoundDuration  int      `json:"round_duration" validate:"required,oneof=30 60 90 120"`
	WinCondition   string   `json:"win_condition" validate:"required,oneof=points rounds"`
	WinValue       int      `json:"win_value" validate:"required,min=1,max=500"`
	PenaltyForSkip bool     `json:"penalty_for_skip"`
	Language       string   `json:"language" validate:"required,oneof=uk en ru"`
	BettingEnabled bool     `json:"betting_enabled"`
	TasksEnabled   bool     `json:"tasks_enabled"`
	Tasks          []string `json:"tasks,omitempty"`
}

func (d *CreateRoomDTO) Bind(r *http.Request) error {
	validate := validator.New()
	return validate.Struct(d)
}

type WordResultDTO struct {
	Result string `json:"result" validate:"required,oneof=correct skip"`
}

func (d *WordResultDTO) Bind(r *http.Request) error {
	validate := validator.New()
	return validate.Struct(d)
}
