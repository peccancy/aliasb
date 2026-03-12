package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/peccancy/aliasb/internal/connection"
	"github.com/peccancy/aliasb/internal/dto"
	"github.com/peccancy/aliasb/internal/model"
	"github.com/peccancy/aliasb/internal/store"
	"github.com/peccancy/aliasb/internal/words"
)

type RoomService struct {
	store         *store.RoomStore
	wordsSvc      *words.Service
	partnerClient *connection.PartnerClient
	partnerID     string
	partnerToken  string
	log           zerolog.Logger
}

func NewRoomService(store *store.RoomStore, wordsSvc *words.Service, partnerClient *connection.PartnerClient, partnerID, partnerToken string, log zerolog.Logger) *RoomService {
	return &RoomService{store: store, wordsSvc: wordsSvc, partnerClient: partnerClient, partnerID: partnerID, partnerToken: partnerToken, log: log}
}

func (s *RoomService) CreateRoom(ctx context.Context, d dto.CreateRoomDTO) (*model.Room, string, error) {
	roomID := uuid.New().String()[:8]
	hostID := uuid.New().String()

	teams := make([]model.Team, d.NumTeams)
	for i := 0; i < d.NumTeams; i++ {
		name := fmt.Sprintf("Team %d", i+1)
		if i < len(d.TeamNames) {
			name = d.TeamNames[i]
		}
		teams[i] = model.Team{
			ID:    uuid.New().String(),
			Name:  name,
			Score: 0,
		}
	}

	room := &model.Room{
		ID:     roomID,
		HostID: hostID,
		Settings: model.RoomSettings{
			NumTeams:       d.NumTeams,
			TeamNames:      d.TeamNames,
			WordsSource:    model.WordsSource(d.WordsSource),
			CustomWords:    d.CustomWords,
			Difficulty:     d.Difficulty,
			RoundDuration:  d.RoundDuration,
			WinCondition:   model.WinCondition(d.WinCondition),
			WinValue:       d.WinValue,
			PenaltyForSkip: d.PenaltyForSkip,
			Language:       d.Language,
			BettingEnabled: d.BettingEnabled,
			TasksEnabled:   d.TasksEnabled,
			Tasks:          d.Tasks,
		},
		Teams:     teams,
		Status:    model.StatusWaiting,
		CreatedAt: time.Now(),
	}

	if d.BettingEnabled && s.partnerClient != nil {
		variants := make([]connection.VariantDTO, len(teams))
		for i, t := range teams {
			variants[i] = connection.VariantDTO{Description: t.Name}
		}
		now := time.Now()
		s.log.Info().Str("partner_id", s.partnerID).Msg("creating dispute")
		disputeID, err := s.partnerClient.CreateDispute(ctx, s.partnerToken, s.partnerID, connection.CreateDisputeRequest{
			Description: "Bet on the winning team",
			MinBet:      1,
			MaxBet:      1000,
			StopDate:    now.Add(2 * time.Hour),
			FinishDate:  now.Add(3 * time.Hour),
			Variants:    variants,
		})
		if err != nil {
			s.log.Error().Err(err).Str("partner_id", s.partnerID).Msg("failed to create dispute, continuing without betting")
		} else {
			room.DisputeID = disputeID
		}
	}

	if err := s.store.Save(ctx, room); err != nil {
		return nil, "", fmt.Errorf("failed to save room: %w", err)
	}

	return room, hostID, nil
}

func (s *RoomService) GetRoom(ctx context.Context, roomID string) (*model.Room, error) {
	return s.store.Get(ctx, roomID)
}

func (s *RoomService) StartGame(ctx context.Context, roomID, hostID string) (*model.Room, error) {
	room, err := s.store.Get(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.HostID != hostID {
		return nil, fmt.Errorf("only host can start game")
	}
	if room.Status != model.StatusWaiting {
		return nil, fmt.Errorf("game already started")
	}

	room.Status = model.StatusPlaying
	round, err := s.createRound(ctx, room)
	if err != nil {
		return nil, err
	}
	room.CurrentRound = round

	if err := s.store.Save(ctx, room); err != nil {
		return nil, fmt.Errorf("failed to save room: %w", err)
	}

	return room, nil
}

func (s *RoomService) ProcessWordResult(ctx context.Context, roomID, result string) (*model.Room, bool, error) {
	room, err := s.store.Get(ctx, roomID)
	if err != nil {
		return nil, false, err
	}
	if room.Status != model.StatusPlaying || room.CurrentRound == nil {
		return nil, false, fmt.Errorf("no active round")
	}
	if room.CurrentRound.Finished {
		return nil, false, fmt.Errorf("round already finished")
	}

	currentWord := room.CurrentRound.CurrentWord()
	if currentWord == "" {
		return nil, false, fmt.Errorf("no more words in round")
	}

	if result == "correct" {
		room.CurrentRound.Correct = append(room.CurrentRound.Correct, currentWord)
		for i := range room.Teams {
			if room.Teams[i].ID == room.CurrentRound.TeamID {
				room.Teams[i].Score++
				break
			}
		}
	} else {
		room.CurrentRound.Skipped = append(room.CurrentRound.Skipped, currentWord)
		if room.Settings.PenaltyForSkip {
			for i := range room.Teams {
				if room.Teams[i].ID == room.CurrentRound.TeamID {
					if room.Teams[i].Score > 0 {
						room.Teams[i].Score--
					}
					break
				}
			}
		}
	}
	room.CurrentRound.CurrentWordIndex++

	roundDone := room.CurrentRound.CurrentWordIndex >= len(room.CurrentRound.Words)

	if err := s.store.Save(ctx, room); err != nil {
		return nil, false, err
	}

	return room, roundDone, nil
}

func (s *RoomService) EndRound(ctx context.Context, roomID string) (*model.Room, bool, error) {
	room, err := s.store.Get(ctx, roomID)
	if err != nil {
		return nil, false, err
	}
	if room.CurrentRound == nil {
		return room, false, nil
	}

	room.CurrentRound.Finished = true
	room.RoundHistory = append(room.RoundHistory, *room.CurrentRound)
	room.CurrentRound = nil

	gameOver := room.IsFinished()
	if gameOver {
		room.Status = model.StatusFinished
	} else {
		// Advance to next team and wait for manual start
		room.CurrentTeamIdx = room.NextTeamIdx()
		room.Status = model.StatusBetweenRounds
	}

	if err := s.store.Save(ctx, room); err != nil {
		return nil, false, err
	}

	return room, gameOver, nil
}

func (s *RoomService) StartNextRound(ctx context.Context, roomID string) (*model.Room, error) {
	room, err := s.store.Get(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.Status != model.StatusBetweenRounds {
		return nil, fmt.Errorf("not in between_rounds state")
	}

	round, err := s.createRound(ctx, room)
	if err != nil {
		return nil, err
	}
	room.CurrentRound = round
	room.Status = model.StatusPlaying

	if err := s.store.Save(ctx, room); err != nil {
		return nil, fmt.Errorf("failed to save room: %w", err)
	}

	return room, nil
}

func (s *RoomService) createRound(ctx context.Context, room *model.Room) (*model.Round, error) {
	var used []string
	for _, r := range room.RoundHistory {
		used = append(used, r.Words...)
	}

	var wordList []string
	if room.Settings.WordsSource == model.WordsCustom && len(room.Settings.CustomWords) > 0 {
		usedSet := make(map[string]bool)
		for _, w := range used {
			usedSet[w] = true
		}
		for _, w := range room.Settings.CustomWords {
			if !usedSet[w] {
				wordList = append(wordList, w)
			}
		}
		if len(wordList) > 20 {
			wordList = wordList[:20]
		}
	} else {
		var err error
		wordList, err = s.wordsSvc.GetWords(room.Settings.Language, room.Settings.Difficulty, 20, used)
		if err != nil {
			return nil, fmt.Errorf("failed to get words: %w", err)
		}
	}

	team := room.Teams[room.CurrentTeamIdx]

	// Pick random task if enabled
	task := ""
	if room.Settings.TasksEnabled && len(room.Settings.Tasks) > 0 {
		task = room.Settings.Tasks[rand.Intn(len(room.Settings.Tasks))]
	}

	roundNum := len(room.RoundHistory) + 1
	return &model.Round{
		Number:           roundNum,
		TeamID:           team.ID,
		Words:            wordList,
		CurrentWordIndex: 0,
		Correct:          []string{},
		Skipped:          []string{},
		Duration:         room.Settings.RoundDuration,
		Task:             task,
	}, nil
}

func (s *RoomService) CountRooms(ctx context.Context) (int64, error) {
	return s.store.Count(ctx)
}

func (s *RoomService) Save(ctx context.Context, room *model.Room) error {
	return s.store.Save(ctx, room)
}

// StopDisputeBetting transitions the room's dispute to game_in_progress status when the game starts.
func (s *RoomService) StopDisputeBetting(ctx context.Context, disputeID string) {
	if s.partnerClient == nil || disputeID == "" {
		return
	}
	if err := s.partnerClient.SetGameInProgress(ctx, s.partnerToken, s.partnerID, disputeID); err != nil {
		s.log.Error().Err(err).Str("dispute_id", disputeID).Msg("failed to set dispute game_in_progress")
	}
}

// SetDisputeWinner sets the winner on the room's dispute when the game ends.
func (s *RoomService) SetDisputeWinner(ctx context.Context, room *model.Room) {
	if s.partnerClient == nil || room.DisputeID == "" {
		return
	}
	winner := room.Winner()
	if winner == nil {
		return
	}
	if err := s.partnerClient.SetDisputeWinner(ctx, s.partnerToken, s.partnerID, room.DisputeID, winner.Name); err != nil {
		s.log.Error().Err(err).Str("dispute_id", room.DisputeID).Str("winner", winner.Name).Msg("failed to set dispute winner")
	}
}

func (s *RoomService) SetDisputeID(ctx context.Context, roomID, disputeID string) error {
	room, err := s.store.Get(ctx, roomID)
	if err != nil {
		return err
	}
	room.DisputeID = disputeID
	return s.store.Save(ctx, room)
}
