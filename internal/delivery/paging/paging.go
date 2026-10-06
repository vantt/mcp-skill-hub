// Package paging provides snapshot-bound, integrity-checked page cursors shared by delivery adapters.
package paging

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	DefaultLimit = 25
	MaximumLimit = 100
)

var cursorMACKey = func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("initialize cursor integrity key: " + err.Error())
	}
	return key
}()

// Page represents a snapshot-bound, integrity-checked page of items.
type Page[T any] struct {
	Items      []T    `json:"items"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	Total      int    `json:"total,omitempty"`
}

type cursorValue struct {
	Version    int    `json:"v"`
	Owner      string `json:"o"`
	FilterHash string `json:"f"`
	LastKey    string `json:"k"`
	Checksum   string `json:"c"`
}

func pageDigest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Owner returns a digest of the filter and sorted result.
func Owner(filter string, sortedResult any) string {
	return pageDigest(struct {
		Filter string `json:"filter"`
		Result any    `json:"result"`
	}{Filter: filter, Result: sortedResult})
}

// EncodeCursor encodes a signed opaque page cursor.
func EncodeCursor(owner, filter, lastKey string) string {
	value := cursorValue{Version: 2, Owner: owner, FilterHash: pageDigest(filter), LastKey: lastKey}
	value.Checksum = cursorChecksum(value)
	data, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(data)
}

func cursorChecksum(value cursorValue) string {
	value.Checksum = ""
	encoded, _ := json.Marshal(value)
	mac := hmac.New(sha256.New, cursorMACKey)
	_, _ = mac.Write([]byte("skillhub-page-cursor-v2\x00"))
	_, _ = mac.Write(encoded)
	return hex.EncodeToString(mac.Sum(nil))
}

// DecodeCursor verifies and decodes an opaque page cursor.
func DecodeCursor(cursor, owner, filter string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > 4096 {
		return "", errors.New("cursor too large")
	}
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	var value cursorValue
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Version != 2 || value.Owner != owner || value.FilterHash != pageDigest(filter) || value.LastKey == "" || !hmac.Equal([]byte(value.Checksum), []byte(cursorChecksum(value))) {
		return "", errors.New("cursor mismatch")
	}
	return value.LastKey, nil
}

// NormalizeLimit bounds the limit to between 1 and MaximumLimit, defaulting to DefaultLimit when 0.
func NormalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultLimit, nil
	}
	if limit < 1 || limit > MaximumLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", MaximumLimit)
	}
	return limit, nil
}

// Make constructs a Page from items and pagination params.
func Make[T any](items []T, limit int, lastKey, owner, filter string, key func(T) string) (Page[T], error) {
	start := 0
	if lastKey != "" {
		found := false
		for index, item := range items {
			if key(item) == lastKey {
				start, found = index + 1, true
				break
			}
		}
		if !found {
			return Page[T]{}, errors.New("cursor last sort key is absent")
		}
	}
	end := min(start+limit, len(items))
	result := Page[T]{Items: append([]T(nil), items[start:end]...), HasMore: end < len(items), Total: len(items)}
	if result.HasMore && len(result.Items) > 0 {
		result.NextCursor = EncodeCursor(owner, filter, key(result.Items[len(result.Items)-1]))
	}
	return result, nil
}
