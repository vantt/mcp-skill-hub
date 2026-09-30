package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

type hardeningBenchmarkCatalog struct {
	skills   []Skill
	snapshot string
}

func (catalog hardeningBenchmarkCatalog) Snapshot() string { return catalog.snapshot }

func (catalog hardeningBenchmarkCatalog) Skills(context.Context) ([]Skill, error) {
	return append([]Skill(nil), catalog.skills...), nil
}

func (catalog hardeningBenchmarkCatalog) Search(_ context.Context, query string, limit int) ([]SearchHit, error) {
	queryTokens := tokenize(query)
	hits := make([]SearchHit, 0, limit)
	for _, skill := range catalog.skills {
		rank := overlap(queryTokens, tokenize(skill.Name+" "+skill.Description+" "+strings.Join(skill.Triggers, " ")))
		if rank > 0 {
			hits = append(hits, SearchHit{SkillID: skill.ID, Rank: rank})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Rank != hits[j].Rank {
			return hits[i].Rank > hits[j].Rank
		}
		return hits[i].SkillID < hits[j].SkillID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func BenchmarkResolverPerOperation(b *testing.B) {
	operationNames := []string{"explore", "design", "implement", "review", "debug", "test", "refactor", "migrate", "document", "operate", "research", "other"}
	skills := make([]Skill, 0, 256)
	for index := 0; index < 256; index++ {
		operation := operationNames[index%len(operationNames)]
		id := fmt.Sprintf("%s-benchmark-%03d", operation, index)
		skills = append(skills, Skill{
			ID: id, CollectionID: "benchmark", Name: strings.ToUpper(operation) + " Specialist",
			Description: "Handle " + operation + " benchmark evidence safely", Status: "active",
			Digest:     "sha256:" + strings.Repeat(fmt.Sprintf("%x", index%16), 64),
			Operations: []string{operation}, Triggers: []string{"handle " + operation + " benchmark evidence"},
			NotFor: []string{"unrelated marketing prose"}, MinScope: "multi_step", Reviewed: true,
		})
	}
	encoded, err := json.Marshal(skills)
	if err != nil {
		b.Fatal(err)
	}
	catalog := hardeningBenchmarkCatalog{skills: skills, snapshot: "sha256:" + strings.Repeat("a", 64)}
	for _, operation := range operationNames {
		b.Run(operation, func(b *testing.B) {
			engine, err := New(catalog, DefaultPolicy(), NewCache(1))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			b.ReportMetric(float64(len(encoded)), "fixture_bytes")
			b.ReportMetric(float64(len(skills)), "skills")
			for index := 0; index < b.N; index++ {
				request := Request{
					SchemaVersion: SchemaVersion,
					RequestID:     fmt.Sprintf("benchmark-%s-%d", operation, index),
					Task: Task{
						Description: fmt.Sprintf("handle %s benchmark evidence variant %d", operation, index%2),
						Scope:       "multi_step",
					},
					Operation: operation,
				}
				response, err := engine.Resolve(context.Background(), request)
				if err != nil || response.Status == "" {
					b.Fatalf("resolve = %#v, %v", response, err)
				}
			}
		})
	}
}
