package render

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func EncodeResponse(w http.ResponseWriter, statusCode int, resp any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(statusCode)
	if resp == nil {
		return
	}
	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		slog.Error("encode response", slog.Any("err", err))
	}
}
