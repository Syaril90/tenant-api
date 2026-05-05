package hub

import (
	"mime/multipart"
	"net/http"
	"strings"

	"modular-api/internal/platform/httpserver"
)

type Handler struct {
	module *Module
}

func (m *Module) Handler() *Handler {
	return &Handler{module: m}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/resident-hub", h.handleCollection)
	mux.HandleFunc("/api/v1/resident-hub/", h.handlePostAction)
}

func (h *Handler) handleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		model, err := h.module.List(ListInput{
			AccountCode: r.URL.Query().Get("accountCode"),
			UnitCode:    r.URL.Query().Get("unitCode"),
		})
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, model)
	case http.MethodPost:
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			httpserver.WriteError(w, http.StatusBadRequest, "invalid multipart form")
			return
		}

		var imageHeader *multipart.FileHeader
		if headers := r.MultipartForm.File["image"]; len(headers) > 0 {
			imageHeader = headers[0]
		}

		item, err := h.module.CreatePost(r.Context(), CreatePostInput{
			AccountCode:   r.FormValue("accountCode"),
			UnitCode:      r.FormValue("unitCode"),
			PostType:      r.FormValue("type"),
			Content:       r.FormValue("content"),
			AvatarURL:     r.FormValue("avatarUrl"),
			ImageHeader:   imageHeader,
			EventTitle:    r.FormValue("eventTitle"),
			EventDate:     r.FormValue("eventDate"),
			EventEndDate:  r.FormValue("eventEndDate"),
			EventLocation: r.FormValue("eventLocation"),
		})
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteCreatedItem(w, "/api/v1/resident-hub/"+item.ID, *item)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

type bodyPayload struct {
	AccountCode string `json:"accountCode"`
	UnitCode    string `json:"unitCode"`
	Content     string `json:"content"`
	AvatarURL   string `json:"avatarUrl"`
}

func (h *Handler) handlePostAction(w http.ResponseWriter, r *http.Request) {
	postID, action, ok := parsePostAction(r.URL.Path)
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid hub route")
		return
	}

	if r.Method != http.MethodPost {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	var payload bodyPayload
	if err := httpserver.Decode(w, r, &payload); err != nil {
		return
	}

	switch action {
	case "replies":
		item, err := h.module.CreateReply(CreateReplyInput{
			AccountCode: payload.AccountCode,
			UnitCode:    payload.UnitCode,
			Content:     payload.Content,
			AvatarURL:   payload.AvatarURL,
		}, postID)
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}
		httpserver.WriteCreatedItem(w, "/api/v1/resident-hub/"+postID, *item)
	case "likes/toggle":
		item, err := h.module.ToggleLike(ToggleLikeInput{
			AccountCode: payload.AccountCode,
			UnitCode:    payload.UnitCode,
		}, postID)
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}
		httpserver.WriteItem(w, *item)
	default:
		httpserver.WriteError(w, http.StatusBadRequest, "unsupported hub action")
	}
}

func parsePostAction(path string) (postID string, action string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/api/v1/resident-hub/")
	if trimmed == "" || trimmed == path {
		return "", "", false
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 || parts[0] == "" {
		return "", "", false
	}

	return parts[0], strings.Join(parts[1:], "/"), true
}
