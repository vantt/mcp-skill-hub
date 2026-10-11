package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

// HTTPBundlePath serves skill bundles on the network listener: one GET returns
// the whole current version of a skill as one archive. The query parameter
// skill names the skill; the ETag is the manifest digest of the version, and
// an If-None-Match naming that digest gets 304 with no body.
const HTTPBundlePath = "/mcp/bundle"

// maxStdioBundleBytes keeps a base64 bundle and its JSON-RPC envelope inside
// one stdio frame.
const maxStdioBundleBytes = (MaxFrameBytes - 64<<10) / 4 * 3

// bundleParams asks for the bundle of a skill over MCP (skills/bundle). Held
// lists the digests the caller already has.
type bundleParams struct {
	mcp.ParamsBase
	SkillID     string   `json:"skill_id"`
	IfNoneMatch []string `json:"if_none_match,omitempty"`
}

type bundleResult struct {
	mcp.ResultBase
	ResultType  string `json:"resultType"`
	SkillID     string `json:"skill_id"`
	Digest      string `json:"digest"`
	NotModified bool   `json:"not_modified,omitempty"`
	MediaType   string `json:"media_type,omitempty"`
	Archive     []byte `json:"archive,omitempty"`
}

// bundleError is the JSON body of a refused bundle request.
type bundleError struct {
	Code          string   `json:"code"`
	Message       string   `json:"message"`
	ReasonCodes   []string `json:"reason_codes,omitempty"`
	ReviewCommand string   `json:"review_command,omitempty"`
	CorrelationID string   `json:"correlation_id,omitempty"`
}

func contentReviewRequiredMessage(skillID string) string {
	return fmt.Sprintf("This skill's content has not been approved, so none of it is readable; do not use the skill and ask the user to run `skillhub skill review %s`.", skillID)
}

// skillBundle is the one code path behind both transports. A delivered or
// confirmed version counts as an activation of the skill.
func (adapter *Server) skillBundle(ctx context.Context, session *mcp.ServerSession, skillID string, held []string) (app.SkillBundle, error) {
	bundle, err := adapter.distribution.SkillBundle(ctx, adapter.workspace, skillID, held)
	if err != nil {
		return bundle, err
	}
	adapter.recordLoad(ctx, session, app.SkillLoad{SkillID: skillID, ResourceKind: "entrypoint", Surface: "skill_get"})
	return bundle, nil
}

// bundleMethod serves skills/bundle over any MCP transport.
func (adapter *Server) bundleMethod(ctx context.Context, session *mcp.ServerSession, params *bundleParams) (*bundleResult, error) {
	if params == nil || strings.TrimSpace(params.SkillID) == "" || len(params.SkillID) > 64 || len(params.IfNoneMatch) > 16 {
		return nil, invalidParams("invalid_request", "A skill_id and at most 16 held digests are required.")
	}
	bundle, err := adapter.skillBundle(ctx, session, strings.TrimSpace(params.SkillID), params.IfNoneMatch)
	if err != nil {
		var review *app.ContentReviewRequiredError
		if errors.As(err, &review) {
			return nil, invalidParams("content_review_required", contentReviewRequiredMessage(review.SkillID))
		}
		return nil, adapter.distributionRPCError(ctx, session, err)
	}
	result := &bundleResult{ResultType: "complete", SkillID: bundle.SkillID, Digest: bundle.Digest, NotModified: bundle.NotModified}
	if !bundle.NotModified {
		if len(bundle.Archive) > maxStdioBundleBytes {
			data, _ := json.Marshal(map[string]string{"code": "bundle_too_large"})
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "The skill bundle is larger than one stdio frame; fetch it over the network listener.", Data: data}
		}
		result.MediaType = skillruntime.BundleMediaType
		result.Archive = bundle.Archive
	}
	return result, nil
}

// bundleHTTPHandler serves HTTPBundlePath behind the listener's gate, which
// has already checked the Host, the token, and the rate limit.
func (adapter *Server) bundleHTTPHandler(server *mcp.Server) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			writeBundleError(writer, http.StatusMethodNotAllowed, bundleError{Code: "method_not_allowed", Message: "Use GET."})
			return
		}
		skillID := strings.TrimSpace(request.URL.Query().Get("skill"))
		if skillID == "" || len(skillID) > 64 {
			writeBundleError(writer, http.StatusBadRequest, bundleError{Code: "invalid_request", Message: "The skill query parameter is required."})
			return
		}
		held := parseIfNoneMatch(request.Header.Get("If-None-Match"))
		session := sessionByID(server, request.Header.Get("Mcp-Session-Id"))
		bundle, err := adapter.skillBundle(request.Context(), session, skillID, held)
		if err != nil {
			status, body := adapter.bundleHTTPError(err)
			writeBundleError(writer, status, body)
			return
		}
		writer.Header().Set("ETag", `"`+bundle.Digest+`"`)
		if bundle.NotModified {
			writer.WriteHeader(http.StatusNotModified)
			return
		}
		writer.Header().Set("Content-Type", skillruntime.BundleMediaType)
		writer.Header().Set("Content-Length", fmt.Sprint(len(bundle.Archive)))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(bundle.Archive)
	})
}

func (adapter *Server) bundleHTTPError(err error) (int, bundleError) {
	var review *app.ContentReviewRequiredError
	switch {
	case errors.As(err, &review):
		return http.StatusForbidden, bundleError{
			Code:          "content_review_required",
			Message:       contentReviewRequiredMessage(review.SkillID),
			ReasonCodes:   review.ReasonCodes,
			ReviewCommand: "skillhub skill review " + review.SkillID,
		}
	case errors.Is(err, skill.ErrNotFound), errors.Is(err, catalog.ErrSkillNotServable), errors.Is(err, skill.ErrSnapshotExpired):
		return http.StatusNotFound, bundleError{Code: "skill_not_found", Message: "The requested skill is not an active, servable skill."}
	case errors.Is(err, catalog.ErrCatalogUnavailable):
		return http.StatusServiceUnavailable, bundleError{Code: "index_stale", Message: "The derived catalog is stale or unavailable on the hub."}
	case errors.Is(err, app.ErrResourceContentUnavailable), errors.Is(err, skill.ErrResourceContentUnavailable), errors.Is(err, skill.ErrResourceDigestMismatch):
		correlation := correlationID()
		adapter.logger.Error("skill bundle content unavailable", "correlation_id", correlation)
		return http.StatusInternalServerError, bundleError{Code: "resource_content_unavailable", Message: "A file of the skill has changed or is missing on the hub.", CorrelationID: correlation}
	default:
		correlation := correlationID()
		adapter.logger.Error("skill bundle internal failure", "correlation_id", correlation)
		return http.StatusInternalServerError, bundleError{Code: "internal_error", Message: "The skill bundle could not be built.", CorrelationID: correlation}
	}
}

func writeBundleError(writer http.ResponseWriter, status int, body bundleError) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

// parseIfNoneMatch returns the digests named by an If-None-Match header. Only
// strong or weak quoted entity tags count; "*" and anything else are ignored,
// so they can only cause a full response.
func parseIfNoneMatch(header string) []string {
	var digests []string
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimPrefix(strings.TrimSpace(part), "W/")
		if len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
			continue
		}
		if digest := tag[1 : len(tag)-1]; skillruntime.ValidBundleDigest(digest) {
			digests = append(digests, digest)
		}
		if len(digests) == 16 {
			break
		}
	}
	return digests
}

// sessionByID finds the MCP session a bundle request belongs to, so the load
// is attributed to the caller's resolutions. An unknown ID yields nil.
func sessionByID(server *mcp.Server, id string) *mcp.ServerSession {
	if id == "" || server == nil {
		return nil
	}
	for session := range server.Sessions() {
		if session.ID() == id {
			return session
		}
	}
	return nil
}

// remoteLocalSkill is the local status a caller on another machine receives:
// never a path on this machine, which it could not use and should not see.
func (adapter *Server) remoteLocalSkill(ctx context.Context, skillID string) *app.LocalSkill {
	local, err := adapter.snapshots.RemoteStatus(ctx, adapter.workspace, skillID)
	if err != nil {
		adapter.logger.Warn("remote skill status could not be read", "skill_id", skillID, "error", err)
		local = app.LocalSkill{Status: app.LocalStatusUnavailable, ReasonCodes: []string{app.LocalReasonExportFailed}}
	}
	return &local
}
