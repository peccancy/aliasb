package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rs/zerolog"

	"github.com/peccancy/aliasb/internal/dto"
	"github.com/peccancy/aliasb/internal/hub"
	"github.com/peccancy/aliasb/internal/response"
	"github.com/peccancy/aliasb/internal/service"
	"github.com/peccancy/aliasb/internal/stats"
)

type RoomHandler struct {
	rooms *service.RoomService
	hub   *hub.Hub
	stats *stats.Store
	log   zerolog.Logger
}

func NewRoomHandler(rooms *service.RoomService, h *hub.Hub, s *stats.Store, log zerolog.Logger) *RoomHandler {
	return &RoomHandler{rooms: rooms, hub: h, stats: s, log: log}
}

func (h *RoomHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var d dto.CreateRoomDTO
	if err := render.Bind(r, &d); err != nil {
		render.Render(w, r, response.ErrBadRequest(err))
		return
	}
	room, hostID, err := h.rooms.CreateRoom(r.Context(), d)
	if err != nil {
		render.Render(w, r, response.ErrInternal(err))
		return
	}
	render.Render(w, r, response.CreateRoomResponse{
		RoomID:         room.ID,
		HostID:         hostID,
		HTTPStatusCode: http.StatusCreated,
	})
}

func (h *RoomHandler) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "id")
	room, err := h.rooms.GetRoom(r.Context(), roomID)
	if err != nil {
		render.Render(w, r, response.ErrNotFound("room not found"))
		return
	}
	render.Render(w, r, response.RoomResponse{Data: room, HTTPStatusCode: http.StatusOK})
}

func (h *RoomHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	count, err := h.rooms.CountRooms(r.Context())
	if err != nil {
		render.Render(w, r, response.ErrInternal(err))
		return
	}
	render.JSON(w, r, map[string]int64{"active_rooms": count})
}

func (h *RoomHandler) StartGame(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "id")
	hostID := r.Header.Get("X-Host-ID")
	if hostID == "" {
		render.Render(w, r, response.ErrBadRequest(fmt.Errorf("X-Host-ID header required")))
		return
	}
	room, err := h.rooms.StartGame(r.Context(), roomID, hostID)
	if err != nil {
		render.Render(w, r, response.ErrBadRequest(err))
		return
	}

	// Stop betting on the dispute if one was created for this room
	if room.DisputeID != "" {
		go h.rooms.StopDisputeBetting(context.Background(), room.DisputeID)
	}

	if h.stats != nil {
		go func() {
			if err := h.stats.RecordGameStarted(context.Background(), room.ID, room.DisputeID != ""); err != nil {
				h.log.Error().Err(err).Str("room_id", room.ID).Msg("failed to record game started")
			}
		}()
	}

	// Broadcast game start to all WS clients
	payload, _ := json.Marshal(room)
	h.hub.BroadcastToRoom(roomID, hub.Message{Type: "room_state", Payload: payload})
	if room.CurrentRound != nil {
		word := room.CurrentRound.CurrentWord()
		wordPayload, _ := json.Marshal(map[string]string{"word": word})
		h.hub.BroadcastToRoom(roomID, hub.Message{Type: "current_word", Payload: wordPayload})
	}
	h.hub.BroadcastToRoom(roomID, hub.Message{Type: "round_started"})

	go func() {
		room2, err := h.rooms.GetRoom(context.Background(), roomID)
		if err != nil {
			return
		}
		p, _ := json.Marshal(room2)
		h.hub.BroadcastToRoom(roomID, hub.Message{Type: "room_state", Payload: p})
	}()

	render.Render(w, r, response.RoomResponse{Data: room, HTTPStatusCode: http.StatusOK})
}
