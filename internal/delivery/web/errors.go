package web

import (
	"encoding/json"
	"net/http"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

var statusByCode = map[app.ErrorCode]int{
	app.ErrorInvalidRequest:             http.StatusBadRequest,          // 400
	app.ErrorValidationFailed:           http.StatusBadRequest,          // 400
	app.ErrorAmbiguousLocator:           http.StatusBadRequest,          // 400
	app.ErrorAmbiguousRef:               http.StatusBadRequest,          // 400
	app.ErrorLocalWatchUnsupported:      http.StatusBadRequest,          // 400
	app.ErrorUnsupportedSchema:          http.StatusBadRequest,          // 400
	app.ErrorResourceLimitsExceeded:      http.StatusBadRequest,          // 400
	app.ErrorSkillSelectionRequired:       http.StatusUnprocessableEntity, // 422
	app.ErrorUnknownResolution:            http.StatusUnprocessableEntity, // 422
	app.ErrorClarificationBudgetExhausted: http.StatusUnprocessableEntity, // 422
	app.ErrorPermissionDenied:             http.StatusForbidden,           // 403
	app.ErrorSkillConflict:                http.StatusConflict,            // 409
	app.ErrorSourceConflict:               http.StatusConflict,            // 409
	app.ErrorEditConflict:                 http.StatusConflict,            // 409
	app.ErrorStaleProposal:                http.StatusConflict,            // 409
	app.ErrorStaleContext:                 http.StatusConflict,            // 409
	app.ErrorSourceChanged:                http.StatusConflict,            // 409
	app.ErrorResolutionRetryExhausted:     http.StatusConflict,            // 409
	app.ErrorResourceContentUnavailable:   http.StatusConflict,            // 409
	app.ErrorSnapshotExpired:              http.StatusGone,                // 410
	app.ErrorWorkspaceInvalid:             http.StatusServiceUnavailable,  // 503
	app.ErrorRecoveryRequired:             http.StatusServiceUnavailable,  // 503
	app.ErrorIndexStale:                   http.StatusServiceUnavailable,  // 503
	app.ErrorSourceUnavailable:            http.StatusBadGateway,          // 502
	app.ErrorResourceReadFailed:           http.StatusBadGateway,          // 502
	app.ErrorOperationCancelled:           499,                            // 499
	app.ErrorInternal:                     http.StatusInternalServerError, // 500
	app.ErrorResourceDigestMismatch:       http.StatusInternalServerError, // 500
	app.ErrorRunInterrupted:               http.StatusInternalServerError, // 500
	app.ErrorPartialDistillFailure:        http.StatusInternalServerError, // 500
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
}

func writeError(w http.ResponseWriter, err error, notFound bool) {
	classified := app.ClassifyError(err)
	status := statusByCode[classified.Code]
	if status == 0 {
		status = http.StatusInternalServerError
	}
	if notFound {
		status = http.StatusNotFound
	}
	writeJSON(w, status, app.ErrorResult(classified))
}

func writeAppError(w http.ResponseWriter, e *app.Error) {
	status := statusByCode[e.Code]
	if status == 0 {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, app.ErrorResult(e))
}
