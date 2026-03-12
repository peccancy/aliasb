package response

import (
	"net/http"

	"github.com/go-chi/render"
	"github.com/peccancy/aliasb/internal/model"
)

type RoomResponse struct {
	Data           *model.Room `json:"data"`
	HTTPStatusCode int         `json:"-"`
}

func (rr RoomResponse) Render(_ http.ResponseWriter, r *http.Request) error {
	render.Status(r, rr.HTTPStatusCode)
	return nil
}

type CreateRoomResponse struct {
	RoomID         string `json:"room_id"`
	HostID         string `json:"host_id"`
	HTTPStatusCode int    `json:"-"`
}

func (rr CreateRoomResponse) Render(_ http.ResponseWriter, r *http.Request) error {
	render.Status(r, rr.HTTPStatusCode)
	return nil
}

type ErrorResponse struct {
	Error          string `json:"error"`
	HTTPStatusCode int    `json:"-"`
}

func (rr ErrorResponse) Render(_ http.ResponseWriter, r *http.Request) error {
	render.Status(r, rr.HTTPStatusCode)
	return nil
}

func ErrBadRequest(err error) render.Renderer {
	return ErrorResponse{Error: err.Error(), HTTPStatusCode: http.StatusBadRequest}
}

func ErrNotFound(msg string) render.Renderer {
	return ErrorResponse{Error: msg, HTTPStatusCode: http.StatusNotFound}
}

func ErrInternal(err error) render.Renderer {
	return ErrorResponse{Error: err.Error(), HTTPStatusCode: http.StatusInternalServerError}
}

func ErrForbidden(msg string) render.Renderer {
	return ErrorResponse{Error: msg, HTTPStatusCode: http.StatusForbidden}
}
