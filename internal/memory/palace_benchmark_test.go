package memory

import (
	"fmt"
	"testing"
)

func benchmarkDocs(n int) []Document {
	docs := make([]Document, 0, n)
	for i := 0; i < n; i++ {
		wing := "internal"
		room := "agent"
		if i%3 == 0 {
			wing = "doc"
			room = "runbooks"
		}
		docs = append(docs, Document{
			ID:      fmt.Sprintf("doc-%d", i),
			Source:  fmt.Sprintf("%s/file-%d.md", wing, i),
			Content: "benchmark payload",
			Palace:  "workspace",
			Wing:    wing,
			Room:    room,
		})
	}
	return docs
}

func BenchmarkPalaceRouting_FilterDocs(b *testing.B) {
	route := PalaceRoute{Palace: "workspace", Wing: "doc"}
	docs := benchmarkDocs(3000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filtered := docs[:0]
		for _, d := range docs {
			if route.MatchesDocument(d) {
				filtered = append(filtered, d)
			}
		}
	}
}

func BenchmarkPalaceRouting_RouteQuery(b *testing.B) {
	router := NewHeuristicPalaceRouter()
	for i := 0; i < b.N; i++ {
		_ = router.Route("optimize memory retrieval for sprint docs")
	}
}
