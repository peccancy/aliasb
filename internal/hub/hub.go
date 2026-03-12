package hub

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Client struct {
	RoomID   string
	PlayerID string
	Conn     *websocket.Conn
	Send     chan []byte
}

type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[string]*Client // roomID -> playerID -> client
	log   zerolog.Logger
}

func NewHub(log zerolog.Logger) *Hub {
	return &Hub{
		rooms: make(map[string]map[string]*Client),
		log:   log,
	}
}

func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[client.RoomID] == nil {
		h.rooms[client.RoomID] = make(map[string]*Client)
	}
	h.rooms[client.RoomID][client.PlayerID] = client
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[client.RoomID]; ok {
		delete(room, client.PlayerID)
		if len(room) == 0 {
			delete(h.rooms, client.RoomID)
		}
	}
}

func (h *Hub) BroadcastToRoom(roomID string, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to marshal ws message")
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.rooms[roomID] {
		select {
		case client.Send <- data:
		default:
			h.log.Warn().Str("player", client.PlayerID).Msg("ws send buffer full, dropping message")
		}
	}
}

func (h *Hub) SendToPlayer(roomID, playerID string, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if room, ok := h.rooms[roomID]; ok {
		if client, ok := room[playerID]; ok {
			select {
			case client.Send <- data:
			default:
			}
		}
	}
}

func (h *Hub) PlayersInRoom(roomID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var ids []string
	for id := range h.rooms[roomID] {
		ids = append(ids, id)
	}
	return ids
}
