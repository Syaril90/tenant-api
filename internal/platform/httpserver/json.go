package httpserver

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(r *http.Request, payload any) error {
	return json.NewDecoder(r.Body).Decode(payload)
}
