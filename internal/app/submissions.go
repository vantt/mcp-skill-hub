package app

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/vantt/skillhub-mcp/internal/skillruntime"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// The queue a hub keeps for skills other machines send for review. It lives
// under runtime/, which the hub's Git never tracks, so a waiting submission
// leaves the hub's tree and HEAD as they are.
const (
	submissionsRelative = "runtime/submissions"
	submissionFileName  = "submission.json"
	submissionFilesDir  = "files"
	submissionStaging   = ".staging"
	// MaxWaitingSubmissions and MaxSubmissionQueueBytes bound what a machine
	// holding the hub token can park on the hub's disk.
	MaxWaitingSubmissions   = 50
	MaxSubmissionQueueBytes = 256 << 20
	// MaxConcurrentSubmissionUploads bounds the uploads still arriving, each
	// of which may hold up to a bundle's size in staging.
	MaxConcurrentSubmissionUploads = 4

	staleStagingAge     = time.Hour
	maxClientLabelRunes = 64
	maxExcludedListed   = 200
	maxExcludedPathLen  = 256
)

var submissionIDPattern = regexp.MustCompile(`^SUB-[0-9A-HJKMNP-TV-Z]{26}$`)

// Errors a submission can fail with. The transport maps them to statuses.
var (
	// ErrSubmissionInvalid reports a submission the hub refuses to queue.
	ErrSubmissionInvalid = errors.New("invalid skill submission")
	// ErrSubmissionQueueFull reports a queue at its count or size cap.
	ErrSubmissionQueueFull = errors.New("the submission queue is full")
	// ErrSubmissionBusy reports too many uploads arriving at once.
	ErrSubmissionBusy = errors.New("the hub is receiving other submissions")
	// ErrSubmissionNotFound reports an id that names no waiting submission.
	ErrSubmissionNotFound = errors.New("no such submission")
)

// SubmissionFile is one content file of a waiting submission.
type SubmissionFile struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Digest     string `json:"digest"`
	Executable bool   `json:"executable,omitempty"`
}

// Submission is a skill another machine sent to this hub for review. It
// changes nothing and serves nothing until a person approves it here.
type Submission struct {
	ID      string `json:"submission_id"`
	SkillID string `json:"skill_id"`
	// BaseVersion is the content digest of the checkout the files came from,
	// or empty for a folder that was never checked out.
	BaseVersion string `json:"base_version,omitempty"`
	// ClientLabel names the submitting machine. It is shown, never trusted.
	ClientLabel string           `json:"client_label"`
	SubmittedAt string           `json:"submitted_at"`
	Excluded    []ExcludedFile   `json:"excluded,omitempty"`
	Files       []SubmissionFile `json:"files"`
	TotalBytes  int64            `json:"total_bytes"`
}

var submissionMu sync.Mutex

func submissionsRoot(root string) string {
	return filepath.Join(root, filepath.FromSlash(submissionsRelative))
}

// SubmissionFilesDir is the folder that holds the files of a waiting submission.
func SubmissionFilesDir(root, id string) string {
	return filepath.Join(submissionsRoot(root), id, submissionFilesDir)
}

// submissionIntake tracks the uploads of one hub that are still arriving: how
// many there are, and the bytes each declared, which count against the queue
// cap before any of them is written.
type submissionIntake struct {
	slots    chan struct{}
	reserved int64
}

var submissionIntakes = map[string]*submissionIntake{}

func intakeOf(root string) *submissionIntake {
	submissionMu.Lock()
	defer submissionMu.Unlock()
	intake := submissionIntakes[root]
	if intake == nil {
		intake = &submissionIntake{slots: make(chan struct{}, MaxConcurrentSubmissionUploads)}
		submissionIntakes[root] = intake
	}
	return intake
}

// queueRoomLocked checks the count and size caps for submission, counting the
// bytes other uploads still arriving have reserved. It returns the waiting
// submissions the new one replaces. submissionMu must be held.
func queueRoomLocked(root string, intake *submissionIntake, submission Submission, ownReserved int64) ([]string, error) {
	existing, err := listSubmissionsLocked(root)
	if err != nil {
		return nil, err
	}
	var replaced []string
	count, total := 0, submission.TotalBytes+intake.reserved-ownReserved
	for _, other := range existing {
		if other.SkillID == submission.SkillID && other.ClientLabel == submission.ClientLabel {
			replaced = append(replaced, other.ID)
			continue
		}
		count++
		total += other.TotalBytes
	}
	if count >= MaxWaitingSubmissions {
		return nil, fmt.Errorf("%w: %d submissions are waiting, the most this hub keeps", ErrSubmissionQueueFull, count)
	}
	if total > MaxSubmissionQueueBytes {
		return nil, fmt.Errorf("%w: waiting submissions would use more than %d MiB", ErrSubmissionQueueFull, MaxSubmissionQueueBytes>>20)
	}
	return replaced, nil
}

// AddSubmission queues the bundle in archive. The files are staged and renamed
// into place in one step, so a reader never sees a half-written submission.
// A waiting submission of the same skill from the same machine is replaced.
// Only a few uploads are taken at once, and an upload that cannot fit under
// the queue caps is refused from its metadata, before any file is written.
func AddSubmission(root string, archive io.Reader, now time.Time) (Submission, error) {
	intake := intakeOf(root)
	select {
	case intake.slots <- struct{}{}:
		defer func() { <-intake.slots }()
	default:
		return Submission{}, fmt.Errorf("%w: %d submissions are arriving, the most this hub takes at once", ErrSubmissionBusy, MaxConcurrentSubmissionUploads)
	}
	queue := submissionsRoot(root)
	staging := filepath.Join(queue, submissionStaging)
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return Submission{}, fmt.Errorf("create the submission queue: %w", err)
	}
	removeStaleStaging(staging, now)
	stage, err := os.MkdirTemp(staging, "incoming-")
	if err != nil {
		return Submission{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	files := filepath.Join(stage, submissionFilesDir)
	if err := os.Mkdir(files, 0o700); err != nil {
		return Submission{}, err
	}
	var submission Submission
	var reserved int64
	defer func() {
		if reserved != 0 {
			submissionMu.Lock()
			intake.reserved -= reserved
			submissionMu.Unlock()
		}
	}()
	_, err = skillruntime.ExtractBundleAdmitting(archive, files, func(metadata skillruntime.BundleMetadata) error {
		described, err := describeSubmission(metadata, now)
		if err != nil {
			return err
		}
		submissionMu.Lock()
		defer submissionMu.Unlock()
		if _, err := queueRoomLocked(root, intake, described, 0); err != nil {
			return err
		}
		submission, reserved = described, described.TotalBytes
		intake.reserved += reserved
		return nil
	})
	if err != nil {
		return Submission{}, err
	}
	if err := os.Remove(filepath.Join(files, skillruntime.BundleMetadataName)); err != nil {
		return Submission{}, err
	}

	submissionMu.Lock()
	defer submissionMu.Unlock()
	replaced, err := queueRoomLocked(root, intake, submission, reserved)
	if err != nil {
		return Submission{}, err
	}
	if err := writeSubmissionRecord(stage, submission); err != nil {
		return Submission{}, err
	}
	if err := os.Rename(stage, filepath.Join(queue, submission.ID)); err != nil {
		return Submission{}, fmt.Errorf("queue the submission: %w", err)
	}
	for _, id := range replaced {
		_ = os.RemoveAll(filepath.Join(queue, id))
	}
	return submission, nil
}

// removeStaleStaging drops uploads a crash left half-written, so they cannot
// pile up on the hub's disk. An upload still arriving is far younger.
func removeStaleStaging(staging string, now time.Time) {
	entries, err := os.ReadDir(staging)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && now.Sub(info.ModTime()) > staleStagingAge {
			_ = os.RemoveAll(filepath.Join(staging, entry.Name()))
		}
	}
}

// describeSubmission checks the extracted bundle's own claims and turns them
// into the record that is kept.
func describeSubmission(metadata skillruntime.BundleMetadata, now time.Time) (Submission, error) {
	invalid := func(format string, args ...any) (Submission, error) {
		return Submission{}, fmt.Errorf("%w: %s", ErrSubmissionInvalid, fmt.Sprintf(format, args...))
	}
	if !skillTargetIDPattern.MatchString(metadata.SkillID) {
		return invalid("%q is not a skill id", metadata.SkillID)
	}
	var details skillruntime.BundleSubmission
	if metadata.Submission != nil {
		details = *metadata.Submission
	}
	if details.BaseVersion != "" && !strings.HasPrefix(details.BaseVersion, "sha256:") {
		return invalid("the base version is not a content digest")
	}
	submission := Submission{
		ID:          newSubmissionID(now),
		SkillID:     metadata.SkillID,
		BaseVersion: details.BaseVersion,
		ClientLabel: cleanClientLabel(details.ClientLabel),
		SubmittedAt: now.UTC().Format("2006-01-02T15:04:05Z"),
		Excluded:    cleanExcluded(details.Excluded),
		Files:       make([]SubmissionFile, 0, len(metadata.Files)),
	}
	folded := make(map[string]string, len(metadata.Files))
	for _, file := range metadata.Files {
		if !printablePath(file.Path) {
			return invalid("the path %q has characters that cannot be shown safely", file.Path)
		}
		lower := strings.ToLower(file.Path)
		if other, twin := folded[lower]; twin {
			return invalid("%q and %q differ only in letter case", other, file.Path)
		}
		folded[lower] = file.Path
		if file.Path == CheckoutMarkerName || file.Path == CheckoutMarkerName+".tmp" || workspace.IsHubMeta(file.Path) {
			return invalid("%s is managed by the hub and cannot be submitted", file.Path)
		}
		if reason, skip := sourcepkg.ExcludedFromImport(file.Path); skip {
			return invalid("%s is never part of a skill (%s)", file.Path, reason)
		}
		submission.Files = append(submission.Files, SubmissionFile{Path: file.Path, Size: file.Size, Digest: file.Digest, Executable: file.Executable})
		submission.TotalBytes += file.Size
	}
	sort.Slice(submission.Files, func(i, j int) bool { return submission.Files[i].Path < submission.Files[j].Path })
	return submission, nil
}

// printablePath reports whether every character of a submitted path is one a
// terminal shows as itself: no control, escape or direction-changing character.
func printablePath(path string) bool {
	for _, r := range path {
		if !unicode.IsPrint(r) && r != '/' {
			return false
		}
	}
	return true
}

// cleanClientLabel keeps a machine label to one short printable line. A label
// over the cap keeps its whole leading dot-separated parts, so a long host name
// never shows a cut-off part; a first part that alone is over the cap is cut
// and ends with an ellipsis.
func cleanClientLabel(label string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(label))
	if cleaned == "" {
		return "unknown machine"
	}
	if utf8.RuneCountInString(cleaned) <= maxClientLabelRunes {
		return cleaned
	}
	parts := strings.Split(strings.TrimLeft(cleaned, "."), ".")
	kept, keptRunes := parts[0], utf8.RuneCountInString(parts[0])
	if keptRunes <= maxClientLabelRunes {
		for _, part := range parts[1:] {
			partRunes := utf8.RuneCountInString(part)
			if keptRunes+1+partRunes > maxClientLabelRunes {
				break
			}
			kept, keptRunes = kept+"."+part, keptRunes+1+partRunes
		}
		if kept == "" {
			return "unknown machine"
		}
		return kept
	}
	return string([]rune(kept)[:maxClientLabelRunes-1]) + "…"
}

func cleanExcluded(excluded []skillruntime.BundleExcluded) []ExcludedFile {
	clean := func(text string) string {
		return strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, text))
	}
	var cleaned []ExcludedFile
	for _, item := range excluded {
		if len(cleaned) == maxExcludedListed {
			break
		}
		path, reason := clean(item.Path), clean(item.Reason)
		if path != "" && len(path) <= maxExcludedPathLen && len(reason) <= maxExcludedPathLen {
			cleaned = append(cleaned, ExcludedFile{Path: path, Reason: reason})
		}
	}
	return cleaned
}

func writeSubmissionRecord(stage string, submission Submission) error {
	encoded, err := json.MarshalIndent(submission, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stage, submissionFileName), append(encoded, '\n'), 0o600)
}

// ListSubmissions returns the waiting submissions, oldest first.
func ListSubmissions(root string) ([]Submission, error) {
	submissionMu.Lock()
	defer submissionMu.Unlock()
	return listSubmissionsLocked(root)
}

func listSubmissionsLocked(root string) ([]Submission, error) {
	entries, err := os.ReadDir(submissionsRoot(root))
	if errors.Is(err, fs.ErrNotExist) {
		return []Submission{}, nil
	}
	if err != nil {
		return nil, err
	}
	submissions := []Submission{}
	for _, entry := range entries {
		if !entry.IsDir() || !submissionIDPattern.MatchString(entry.Name()) {
			continue
		}
		submission, err := readSubmissionRecord(root, entry.Name())
		if err != nil {
			// A record that cannot be read is not waiting for anyone.
			continue
		}
		submissions = append(submissions, submission)
	}
	sort.Slice(submissions, func(i, j int) bool { return submissions[i].ID < submissions[j].ID })
	return submissions, nil
}

// ReadSubmission returns one waiting submission.
func ReadSubmission(root, id string) (Submission, error) {
	if !submissionIDPattern.MatchString(id) {
		return Submission{}, fmt.Errorf("%w: %q is not a submission id", ErrSubmissionNotFound, id)
	}
	return readSubmissionRecord(root, id)
}

func readSubmissionRecord(root, id string) (Submission, error) {
	encoded, err := os.ReadFile(filepath.Join(submissionsRoot(root), id, submissionFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return Submission{}, fmt.Errorf("%w: %s", ErrSubmissionNotFound, id)
	}
	if err != nil {
		return Submission{}, err
	}
	var submission Submission
	if err := json.Unmarshal(encoded, &submission); err != nil || submission.ID != id {
		return Submission{}, fmt.Errorf("the submission record %s cannot be read", id)
	}
	return submission, nil
}

// DiscardSubmission drops a waiting submission without applying it.
func DiscardSubmission(root, id string) error {
	if _, err := ReadSubmission(root, id); err != nil {
		return err
	}
	submissionMu.Lock()
	defer submissionMu.Unlock()
	return os.RemoveAll(filepath.Join(submissionsRoot(root), id))
}

var proposalIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

func proposalBindingPath(root, id, proposalID string) string {
	return filepath.Join(submissionsRoot(root), id, "proposal-"+proposalID)
}

// BindSubmissionProposal records that a proposal was made from a submission,
// so confirming that proposal, and only that one, may drop the submission.
func BindSubmissionProposal(root, id, proposalID string) error {
	if _, err := ReadSubmission(root, id); err != nil {
		return err
	}
	if !proposalIDPattern.MatchString(proposalID) {
		return fmt.Errorf("%q is not a proposal id", proposalID)
	}
	return os.WriteFile(proposalBindingPath(root, id, proposalID), nil, 0o600)
}

// SubmissionBoundToProposal reports whether proposalID was made from the
// waiting submission id.
func SubmissionBoundToProposal(root, id, proposalID string) bool {
	if !submissionIDPattern.MatchString(id) || !proposalIDPattern.MatchString(proposalID) {
		return false
	}
	_, err := os.Stat(proposalBindingPath(root, id, proposalID))
	return err == nil
}

// readSubmissionTree reads the files of a waiting submission, checking each
// against the size and digest recorded when it arrived.
func readSubmissionTree(root string, submission Submission) (map[string]treeFile, error) {
	tree := make(map[string]treeFile, len(submission.Files))
	dir := SubmissionFilesDir(root, submission.ID)
	for _, file := range submission.Files {
		if !safeTreePath(file.Path) {
			return nil, fmt.Errorf("the submission lists %q, which leaves the skill folder", file.Path)
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file.Path)))
		if err != nil {
			return nil, fmt.Errorf("read submitted file %s: %w", file.Path, err)
		}
		if int64(len(data)) != file.Size || sourcepkg.Digest(data) != file.Digest {
			return nil, fmt.Errorf("the submitted file %s changed on disk after it arrived", file.Path)
		}
		tree[file.Path] = treeFile{Data: data, Executable: file.Executable}
	}
	return tree, nil
}

// submissionEntries lists the waiting submissions for the review queue.
func submissionEntries(root string) []Submission {
	submissions, err := ListSubmissions(root)
	if err != nil {
		return nil
	}
	return submissions
}

var (
	idClockMu    sync.Mutex
	lastIDMillis uint64
)

// crockford is the ULID alphabet.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newSubmissionID makes a sortable id: 48 bits of time and 80 random bits, in
// the 26 characters of a ULID, so ids order by arrival.
func newSubmissionID(now time.Time) string {
	var data [16]byte
	// Two ids made in one millisecond still order by arrival.
	idClockMu.Lock()
	millis := uint64(now.UnixMilli())
	if millis <= lastIDMillis {
		millis = lastIDMillis + 1
	}
	lastIDMillis = millis
	idClockMu.Unlock()
	for index := 0; index < 6; index++ {
		data[index] = byte(millis >> (8 * (5 - index)))
	}
	if _, err := rand.Read(data[6:]); err != nil {
		for index := 6; index < 16; index++ {
			data[index] = byte(now.UnixNano() >> (index % 8))
		}
	}
	encoded := make([]byte, 26)
	// 128 bits into 26 groups of 5 bits, the first group holding the top 3.
	for group := 0; group < 26; group++ {
		shift := 125 - 5*group
		var value byte
		for bit := 0; bit < 5; bit++ {
			position := shift + bit
			if position < 0 || position > 127 {
				continue
			}
			byteIndex := 15 - position/8
			if data[byteIndex]>>(position%8)&1 == 1 {
				value |= 1 << bit
			}
		}
		encoded[group] = crockford[value]
	}
	return "SUB-" + string(encoded)
}
