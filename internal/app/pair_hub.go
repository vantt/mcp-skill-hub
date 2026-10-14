package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/vantt/skillhub-mcp/internal/hubtoken"
)

// HubPairPath is the hub's route that trades a one-time invite for the hub
// token.
const HubPairPath = "/mcp/pair"

// pairTimeout bounds one pairing request; tests shorten it.
var pairTimeout = 10 * time.Second

// pairBodyLimit caps what is read from the hub's answer.
const pairBodyLimit = 4096

// PairHub trades an invite for the hub's token: it posts the code to the
// hub's pairing route and returns the token the hub answers with. The request
// ignores proxy settings and does not follow redirects, so the code and the
// token only ever travel to the hub's own address. hubURL is a normalised
// hub URL. Neither the code nor the token appears in an error.
func PairHub(ctx context.Context, hubURL, invite string) (string, error) {
	body, err := json.Marshal(struct {
		Invite string `json:"invite"`
	}{invite})
	if err != nil {
		return "", err
	}
	client := hubProbeClient()
	client.Timeout = pairTimeout
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, hubURL+HubPairPath, bytes.NewReader(body))
	if err != nil {
		return "", pairFailure("The hub URL cannot be used.", "The request to "+hubURL+" cannot be built.", "Pass the base URL the hub printed when it started.")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		reason := networkReason(err)
		if os.IsTimeout(err) || errors.Is(err, context.DeadlineExceeded) {
			reason = fmt.Sprintf("no answer within %d seconds", int(pairTimeout.Seconds()))
		}
		return "", pairFailure("The invite could not be sent.", "The hub does not answer at "+hubURL+" right now ("+reason+").", "Start `skillhub serve` on the hub machine (and open the SSH tunnel if you use one), then run this command again. The invite is still valid until it expires.")
	}
	defer response.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(response.Body, pairBodyLimit))
	switch {
	case response.StatusCode == http.StatusOK:
		var reply struct {
			Token string `json:"token"`
		}
		if json.Unmarshal(answer, &reply) != nil || !hubtoken.ValidToken(reply.Token) {
			return "", pairFailure("The hub did not send a usable token.", "The answer from "+hubURL+" is not what a Skill Hub sends.", "Check that the URL is the hub's own, then ask for a new invite with `skillhub token invite` on the hub machine.")
		}
		return reply.Token, nil
	case response.StatusCode == http.StatusForbidden && strings.Contains(string(answer), "Tailscale or an SSH tunnel"):
		return "", pairFailure("The hub refused the invite.", "pairing is only offered over Tailscale or an SSH tunnel.", "Use the Tailscale address of the hub or an SSH tunnel, or use the scp line from `skillhub serve`.")
	case response.StatusCode == http.StatusForbidden:
		return "", pairFailure("The invite was not accepted.", "this invite is not valid: it expired, was already used, or is wrong.", "On the hub machine run `skillhub token invite` for a new one.")
	case response.StatusCode == http.StatusNotFound:
		return "", pairFailure("The hub does not take invites.", "this hub does not offer invites.", "Use the scp line from `skillhub serve` to copy the token file.")
	case response.StatusCode == http.StatusTooManyRequests:
		return "", pairFailure("The hub is not taking attempts now.", "the hub is refusing attempts for a minute.", "Wait a minute, then run this command again.")
	case response.StatusCode >= 500:
		return "", pairFailure("The hub could not take the invite.", fmt.Sprintf("the hub answered with an error (HTTP %d).", response.StatusCode), "Check that `skillhub serve` is running on the hub machine, then run this command again. If the invite was used up, ask for a new one with `skillhub token invite`.")
	}
	return "", pairFailure("The answer is not from a Skill Hub that offers invites.", fmt.Sprintf("something at %s answered with HTTP %d.", hubURL, response.StatusCode), "Check the hub URL, or use the scp line from `skillhub serve`.")
}

func pairFailure(what, why, fix string) *Error {
	return &Error{Code: ErrorInvalidRequest, Render: ErrorRender{Error: what, Why: why, Fix: fix}}
}
