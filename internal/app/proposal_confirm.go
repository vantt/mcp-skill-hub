package app

import "time"

// proposalRefusal names why supplied confirmation pins cannot confirm a reviewed proposal.
type proposalRefusal int

const (
	proposalAccepted proposalRefusal = iota
	proposalExpired
	proposalDigestMismatch
	proposalPinsMismatch
)

// verifyProposalPins decides whether supplied pins confirm the reviewed proposal.
// Checks run in this order: expiry, digest, remaining pins.
// zeroExpiryIsExpired preserves each caller's policy for proposals without an expiry time;
// pass checkExpiry=false when the caller has already enforced expiry.
func verifyProposalPins(now, expiresAt time.Time, checkExpiry, zeroExpiryIsExpired bool, expected, supplied ConfirmationPins) proposalRefusal {
	if checkExpiry {
		if expiresAt.IsZero() {
			if zeroExpiryIsExpired {
				return proposalExpired
			}
		} else if !now.Before(expiresAt) {
			return proposalExpired
		}
	}
	if expected.ProposalDigest != supplied.ProposalDigest {
		return proposalDigestMismatch
	}
	if expected.ProposalID != supplied.ProposalID || expected.BaseVersion != supplied.BaseVersion {
		return proposalPinsMismatch
	}
	return proposalAccepted
}
