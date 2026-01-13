package httperr

import (
	"encoding/json"
	"net/http"
)

type Code string

const (
	InvalidQuery       Code = "INVALID_QUERY"
	RateLimitExceeded  Code = "RATE_LIMIT_EXCEEDED"
	UpstreamOverloaded Code = "UPSTREAM_OVERLOADED"
	GBIFTimeout        Code = "GBIF_TIMEOUT"
	GBIFUnavailable    Code = "GBIF_UNAVAILABLE"
	Internal           Code = "INTERNAL_ERROR"
)

type Response struct {
	Error Detail `json:"error"`
}

type Detail struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func Write(w http.ResponseWriter, status int, code Code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{
		Error: Detail{
			Code:    code,
			Message: message,
		},
	})
}

func InvalidQueryError(w http.ResponseWriter, message string) {
	Write(w, http.StatusBadRequest, InvalidQuery, message)
}

func RateLimitError(w http.ResponseWriter) {
	Write(w, http.StatusTooManyRequests, RateLimitExceeded, "Too many requests")
}

func UpstreamOverloadedError(w http.ResponseWriter) {
	Write(w, http.StatusServiceUnavailable, UpstreamOverloaded, "Upstream service is overloaded")
}

func GBIFTimeoutError(w http.ResponseWriter) {
	Write(w, http.StatusGatewayTimeout, GBIFTimeout, "GBIF request timed out")
}

func GBIFUnavailableError(w http.ResponseWriter) {
	Write(w, http.StatusBadGateway, GBIFUnavailable, "GBIF service is unavailable")
}

func InternalError(w http.ResponseWriter) {
	Write(w, http.StatusInternalServerError, Internal, "Internal server error")
}
