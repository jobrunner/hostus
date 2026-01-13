package api

import (
	"github.com/jobrunner/hostus/internal/httperr"
)

// Re-export error types for convenience
type ErrorCode = httperr.Code
type ErrorResponse = httperr.Response
type ErrorDetail = httperr.Detail

const (
	ErrInvalidQuery       = httperr.InvalidQuery
	ErrRateLimitExceeded  = httperr.RateLimitExceeded
	ErrUpstreamOverloaded = httperr.UpstreamOverloaded
	ErrGBIFTimeout        = httperr.GBIFTimeout
	ErrGBIFUnavailable    = httperr.GBIFUnavailable
	ErrInternal           = httperr.Internal
)

// Re-export error functions
var (
	WriteError             = httperr.Write
	InvalidQueryError      = httperr.InvalidQueryError
	RateLimitError         = httperr.RateLimitError
	UpstreamOverloadedError = httperr.UpstreamOverloadedError
	GBIFTimeoutError       = httperr.GBIFTimeoutError
	GBIFUnavailableError   = httperr.GBIFUnavailableError
	InternalError          = httperr.InternalError
)
