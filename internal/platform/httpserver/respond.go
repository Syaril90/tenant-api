package httpserver

import (
	"errors"
	"net/http"

	"modular-api/internal/platform/apperrors"
)

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

type ItemResponse[T any] struct {
	Item T `json:"item"`
}

func WriteOK(w http.ResponseWriter, payload any) {
	writeJSON(w, http.StatusOK, payload)
}

func WriteItem[T any](w http.ResponseWriter, item T) {
	writeJSON(w, http.StatusOK, ItemResponse[T]{Item: item})
}

func WriteCreated(w http.ResponseWriter, location string, payload any) {
	if location != "" {
		w.Header().Set("Location", location)
	}
	writeJSON(w, http.StatusCreated, payload)
}

func WriteCreatedItem[T any](w http.ResponseWriter, location string, item T) {
	if location != "" {
		w.Header().Set("Location", location)
	}
	writeJSON(w, http.StatusCreated, ItemResponse[T]{Item: item})
}

func WriteError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{
		Code:    errorCodeForStatus(status),
		Message: message,
		Error:   message,
	})
}

func WriteMappedError(w http.ResponseWriter, err error) {
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		switch appErr.Kind {
		case apperrors.KindValidation:
			WriteError(w, http.StatusBadRequest, appErr.Message)
		case apperrors.KindNotFound:
			WriteError(w, http.StatusNotFound, appErr.Message)
		case apperrors.KindConflict:
			WriteError(w, http.StatusConflict, appErr.Message)
		default:
			WriteError(w, http.StatusInternalServerError, appErr.Message)
		}
		return
	}

	WriteError(w, http.StatusInternalServerError, "internal server error")
}

func WriteMethodNotAllowed(w http.ResponseWriter) {
	WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func Decode(w http.ResponseWriter, r *http.Request, payload any) error {
	if err := decodeJSON(r, payload); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json body")
		return err
	}
	return nil
}

func errorCodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	default:
		return "internal_error"
	}
}
