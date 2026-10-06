package paging_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
)

func TestOpaqueCursorMultiPageAndIntegrity(t *testing.T) {
	t.Parallel()
	items := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	filter := "status=active"
	owner := paging.Owner(filter, items)
	cursor := ""
	var collected []string
	for {
		lastKey, err := paging.DecodeCursor(cursor, owner, filter)
		if err != nil {
			t.Fatal(err)
		}
		paged, err := paging.Make(items, 2, lastKey, owner, filter, func(item string) string { return item })
		if err != nil {
			t.Fatal(err)
		}
		collected = append(collected, paged.Items...)
		if !paged.HasMore {
			break
		}
		cursor = paged.NextCursor
	}
	if strings.Join(collected, ",") != strings.Join(items, ",") {
		t.Fatalf("collected pages = %#v", collected)
	}
	cursor = paging.EncodeCursor(owner, filter, "bravo")
	mutated := append(append([]string(nil), items...), "foxtrot")
	if _, err := paging.DecodeCursor(cursor, paging.Owner(filter, mutated), filter); err == nil {
		t.Fatal("cursor survived owner-result mutation")
	}
	bytes, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatal(err)
	}
	bytes[len(bytes)-2] ^= 1
	if _, err := paging.DecodeCursor(base64.RawURLEncoding.EncodeToString(bytes), owner, filter); err == nil {
		t.Fatal("tampered cursor passed checksum validation")
	}
}
