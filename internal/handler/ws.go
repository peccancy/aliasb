package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/peccancy/aliasb/internal/dto"
	"github.com/peccancy/aliasb/internal/hub"
	"github.com/peccancy/aliasb/internal/model"
	"github.com/peccancy/aliasb/internal/service"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WSHandler struct {
	hub   *hub.Hub
	rooms *service.RoomService
	log   zerolog.Logger
}

func NewWSHandler(h *hub.Hub, rooms *service.RoomService, log zerolog.Logger) *WSHandler {
	return &WSHandler{hub: h, rooms: rooms, log: log}
}

type incomingMsg struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "id")

	// Verify room exists
	if _, err := h.rooms.GetRoom(r.Context(), roomID); err != nil {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	// Use provided client_id or generate one
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		clientID = uuid.New().String()
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error().Err(err).Msg("ws upgrade failed")
		return
	}

	client := &hub.Client{
		RoomID:   roomID,
		PlayerID: clientID,
		Conn:     conn,
		Send:     make(chan []byte, 256),
	}
	h.hub.Register(client)
	defer func() {
		h.hub.Unregister(client)
		conn.Close()
	}()

	// Send current room state on connect
	h.broadcastRoomState(roomID)

	// Write pump
	go func() {
		ticker := time.NewTicker(54 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case msg, ok := <-client.Send:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if !ok {
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read pump
	conn.SetReadLimit(512)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, msgData, err := conn.ReadMessage()
		if err != nil {
			break
		}
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		h.handleMessage(roomID, msgData)
	}
}

func (h *WSHandler) handleMessage(roomID string, data []byte) {
	var msg incomingMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "word_result":
		var d dto.WordResultDTO
		if err := json.Unmarshal(msg.Payload, &d); err != nil {
			return
		}
		room, roundDone, err := h.rooms.ProcessWordResult(context.Background(), roomID, d.Result)
		if err != nil {
			h.log.Error().Err(err).Msg("process word result failed")
			return
		}
		if roundDone {
			h.broadcastRoomState(roomID)
			gameRoom, gameOver, err := h.rooms.EndRound(context.Background(), roomID)
			if err != nil {
				h.log.Error().Err(err).Msg("auto end round failed")
				return
			}
			if gameOver {
				winner := gameRoom.Winner()
				payload, _ := json.Marshal(map[string]interface{}{"winner": winner, "teams": gameRoom.Teams})
				h.hub.BroadcastToRoom(roomID, hub.Message{Type: "game_over", Payload: payload})
				go h.rooms.SetDisputeWinner(context.Background(), gameRoom)
			} else {
				h.broadcastRoomState(roomID)
			}
			return
		}
		// Broadcast updated state and next word to all
		h.broadcastRoomState(roomID)
		if room.CurrentRound != nil && !room.CurrentRound.Finished {
			word := room.CurrentRound.CurrentWord()
			payload, _ := json.Marshal(map[string]string{"word": word})
			h.hub.BroadcastToRoom(roomID, hub.Message{Type: "current_word", Payload: payload})
		}

	case "end_round":
		room, err := h.rooms.GetRoom(context.Background(), roomID)
		if err != nil {
			return
		}
		if room.CurrentRound == nil || room.CurrentRound.Finished {
			return
		}
		gameRoom, gameOver, err := h.rooms.EndRound(context.Background(), roomID)
		if err != nil {
			h.log.Error().Err(err).Msg("end round failed")
			return
		}
		if gameOver {
			winner := gameRoom.Winner()
			payload, _ := json.Marshal(map[string]interface{}{"winner": winner, "teams": gameRoom.Teams})
			h.hub.BroadcastToRoom(roomID, hub.Message{Type: "game_over", Payload: payload})
			go h.rooms.SetDisputeWinner(context.Background(), gameRoom)
		} else {
			h.broadcastRoomState(roomID)
		}

	case "end_game":
		room, err := h.rooms.GetRoom(context.Background(), roomID)
		if err != nil {
			return
		}
		room.Status = model.StatusFinished
		if room.CurrentRound != nil {
			room.CurrentRound.Finished = true
			room.RoundHistory = append(room.RoundHistory, *room.CurrentRound)
			room.CurrentRound = nil
		}
		if err := h.rooms.Save(context.Background(), room); err != nil {
			h.log.Error().Err(err).Msg("end game save failed")
			return
		}
		winner := room.Winner()
		payload, _ := json.Marshal(map[string]interface{}{"winner": winner, "teams": room.Teams})
		h.hub.BroadcastToRoom(roomID, hub.Message{Type: "game_over", Payload: payload})
		go h.rooms.SetDisputeWinner(context.Background(), room)

	case "start_round":
		gameRoom, err := h.rooms.StartNextRound(context.Background(), roomID)
		if err != nil {
			h.log.Error().Err(err).Msg("start next round failed")
			return
		}
		h.broadcastRoomState(roomID)
		h.hub.BroadcastToRoom(roomID, hub.Message{Type: "round_started"})
		if gameRoom.CurrentRound != nil {
			word := gameRoom.CurrentRound.CurrentWord()
			payload, _ := json.Marshal(map[string]string{"word": word})
			h.hub.BroadcastToRoom(roomID, hub.Message{Type: "current_word", Payload: payload})
		}
	}
}

func (h *WSHandler) broadcastRoomState(roomID string) {
	room, err := h.rooms.GetRoom(context.Background(), roomID)
	if err != nil {
		return
	}
	payload, _ := json.Marshal(room)
	h.hub.BroadcastToRoom(roomID, hub.Message{Type: "room_state", Payload: payload})
}
