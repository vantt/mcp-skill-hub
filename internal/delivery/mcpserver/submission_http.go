package mcpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/skillruntime"
)

// HTTPSubmissionsPath receives a skill another machine sends for review. The
// body is a bundle archive; the reply names the queue entry. A submission only
// waits in a queue on the hub: nothing changes and nothing is served until a
// person on the hub machine approves it.
const HTTPSubmissionsPath = "/mcp/submissions"

type submissionReply struct {
	SubmissionID string `json:"submission_id"`
	Status       string `json:"status"`
}

// submissionHTTPHandler serves HTTPSubmissionsPath behind the listener's gate,
// which has already checked the Host, the token and the rate limit.
func (adapter *Server) submissionHTTPHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			writeBundleError(writer, http.StatusMethodNotAllowed, bundleError{Code: "method_not_allowed", Message: "Use POST."})
			return
		}
		if request.Header.Get("Content-Type") != skillruntime.BundleMediaType {
			writeBundleError(writer, http.StatusUnsupportedMediaType, bundleError{Code: "invalid_request", Message: "Send the skill as " + skillruntime.BundleMediaType + "."})
			return
		}
		body := http.MaxBytesReader(writer, request.Body, skillruntime.BundleMaxArchiveBytes)
		submission, err := app.AddSubmission(adapter.workspace, body, time.Now())
		if err != nil {
			status, reply := adapter.submissionHTTPError(err)
			writeBundleError(writer, status, reply)
			return
		}
		adapter.logger.Info("skill submission queued", "submission_id", submission.ID, "skill_id", submission.SkillID)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(writer).Encode(submissionReply{SubmissionID: submission.ID, Status: "waiting"})
	})
}

func (adapter *Server) submissionHTTPError(err error) (int, bundleError) {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, skillruntime.ErrBundleLimit), errors.As(err, &tooLarge):
		return http.StatusRequestEntityTooLarge, bundleError{
			Code:    "resource_limits_exceeded",
			Message: "The skill is larger than a skill may be: at most 512 files and 16 MiB in total.",
		}
	case errors.Is(err, app.ErrSubmissionQueueFull):
		return http.StatusTooManyRequests, bundleError{Code: "submission_queue_full", Message: "The hub's review queue is full. " + err.Error() + ". Ask the person who runs the hub to review or discard waiting submissions."}
	case errors.Is(err, skillruntime.ErrBundleInvalid), errors.Is(err, skillruntime.ErrBundleDigestMismatch), errors.Is(err, app.ErrSubmissionInvalid):
		return http.StatusBadRequest, bundleError{Code: "invalid_request", Message: err.Error()}
	}
	correlation := correlationID()
	adapter.logger.Error("skill submission internal failure", "correlation_id", correlation, "error", err)
	return http.StatusInternalServerError, bundleError{Code: "internal_error", Message: "The submission could not be queued.", CorrelationID: correlation}
}
