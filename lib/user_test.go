package korra

import (
	"bufio"
	"strings"
	"testing"
)

func TestScanTargetsToChunks(t *testing.T) {
	raw := `
GET /foo/bar
Header:Value
// this is a comment
POST /foo/bar/baz
Header:Value
Header-Two:Value
@path/to/body
POST /buzzer
Header:Bees
Header-Two:Honey
@path/to/hive
=> PAUSE 12345
HEAD /foos
`
	expected := []string{
		"GET /foo/bar\nHeader:Value",
		"POST /foo/bar/baz\nHeader:Value\nHeader-Two:Value\n@path/to/body",
		"POST /buzzer\nHeader:Bees\nHeader-Two:Honey\n@path/to/hive",
		"=> PAUSE 12345",
		"HEAD /foos",
	}
	scanner := bufio.NewScanner(strings.NewReader(raw))
	chunks := ScanTargetsToChunks(peekingScanner{src: scanner})
	if len(expected) != len(chunks) {
		t.Fatalf("Expected %d chunks, got %d", len(expected), len(chunks))
	}
	for idx, want := range expected {
		if want != chunks[idx] {
			t.Fatalf("Chunk %d; expected %s, got %s", idx, want, chunks[idx])
		}
	}
}
