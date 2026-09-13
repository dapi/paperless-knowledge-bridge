package bridge

import "testing"

func TestSearchCapsResultsAndExcerpts(t *testing.T) {
	s, err := OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 8; i++ {
		if err := s.ReplaceDocument(i, []Chunk{{DocumentID: i, Title: "document", Text: string(make([]rune, MaxExcerptRunes+10)), Embedding: []float64{float64(i), 1}}}); err != nil {
			t.Fatal(err)
		}
	}
	got := s.Search([]float64{1, 1}, 99)
	if len(got) != MaxResults {
		t.Fatalf("got %d results", len(got))
	}
	for _, result := range got {
		if len([]rune(result.Excerpt)) > MaxExcerptRunes+1 {
			t.Fatalf("excerpt too long: %d", len([]rune(result.Excerpt)))
		}
	}
}

func TestDeleteMissingRemovesAllChunks(t *testing.T) {
	s, _ := OpenStore("")
	_ = s.ReplaceDocument(1, []Chunk{{DocumentID: 1}, {DocumentID: 1}})
	_ = s.ReplaceDocument(2, []Chunk{{DocumentID: 2}})
	if err := s.DeleteMissing(map[int]bool{2: true}); err != nil {
		t.Fatal(err)
	}
	if len(s.data.Chunks) != 1 || s.data.Chunks[key(2, 0)].DocumentID != 2 {
		t.Fatalf("unexpected chunks: %#v", s.data.Chunks)
	}
}

func TestSplitPreservesFullText(t *testing.T) {
	parts := Split("abcdef", 4, 1)
	if len(parts) != 2 || parts[0] != "abcd" || parts[1] != "def" {
		t.Fatalf("unexpected split: %#v", parts)
	}
}
