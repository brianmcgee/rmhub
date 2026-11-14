package hashtree

import (
	"os"
	"strings"
	"testing"
)

var (
	h1 = strings.Repeat("1", 64)
	h2 = strings.Repeat("2", 64)
	h3 = strings.Repeat("3", 64)
)

func mustParse(t *testing.T, s string) *Index {
	t.Helper()
	ix, err := Parse([]byte(s))
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ix
}

func TestParseSchema3Document(t *testing.T) {
	ix := mustParse(t, "3\n"+h1+":0:d1.metadata:0:120\n"+h2+":0:d1.content:0:800\n")
	if ix.Schema != 3 || len(ix.Entries) != 2 {
		t.Fatalf("%+v", ix)
	}
	want := Entry{Hash: h2, Type: "0", Name: "d1.content", Subfiles: 0, Size: 800}
	if ix.Entries[1] != want {
		t.Fatalf("entry %+v, want %+v", ix.Entries[1], want)
	}
}

func TestParseSchema4Root(t *testing.T) {
	ix := mustParse(t, "4\n0:.:1:920\n"+h3+":0:d1:2:920\n")
	if ix.Schema != 4 || len(ix.Entries) != 1 || ix.Entries[0].Name != "d1" || ix.Entries[0].Subfiles != 2 {
		t.Fatalf("%+v", ix)
	}
}

func TestParseErrors(t *testing.T) {
	for name, s := range map[string]string{
		"four fields":    "3\n" + h1 + ":0:d1.metadata:0\n",
		"unknown schema": "5\n" + h1 + ":0:d1:0:1\n",
		"empty":          "\n\n",
		"no summary":     "4\n",
		"bad summary":    "4\n1:.:1:1\n",
		"short hash":     "3\nabc:0:d1:0:1\n",
		"upper hash":     "3\n" + strings.Repeat("A", 64) + ":0:d1:0:1\n",
		"bad size":       "3\n" + h1 + ":0:d1:0:x\n",
		"bad subfiles":   "3\n" + h1 + ":0:d1:-1:1\n",
	} {
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("%s: parse succeeded", name)
		}
	}
	_, err := Parse([]byte("3\n" + h2 + ":0:ok:0:1\n" + h1 + ":0:d1.metadata:0\n"))
	if err == nil || !strings.Contains(err.Error(), "d1.metadata:0") || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("error does not name the line: %v", err)
	}
}

func TestLenient(t *testing.T) {
	ix := mustParse(t, "3\n\n"+h1+":80000000:d1:4:10\n\n")
	if len(ix.Entries) != 1 || ix.Entries[0].Type != TypeDoc {
		t.Fatalf("%+v", ix)
	}
	ix = mustParse(t, "4\n0:.:5:920\n"+h3+":0:d1:2:920\n")
	if len(ix.Entries) != 1 {
		t.Fatalf("%+v", ix)
	}
	// Schema 4 with the schema 3 root type, and no final newline.
	ix = mustParse(t, "4\n0:.:1:1\n"+h3+":80000000:d1:2:1")
	if len(ix.Entries) != 1 {
		t.Fatalf("%+v", ix)
	}
}

func TestFileHash(t *testing.T) {
	if got := FileHash([]byte("test")); got != "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Fatal(got)
	}
}

func TestEntryListHashOrder(t *testing.T) {
	a := mustParse(t, "3\n"+h1+":0:d1.metadata:0:1\n"+h2+":0:d1.content:0:2\n")
	b := mustParse(t, "3\n"+h2+":0:d1.content:0:2\n"+h1+":0:d1.metadata:0:1\n")
	if a.DocumentHash() != b.DocumentHash() {
		t.Fatal("entry order changes the hash")
	}
	// The hash is over the hashes sorted by name: content (h2), then metadata (h1).
	raw := make([]byte, 0, 64)
	for _, c := range []byte{0x22, 0x11} {
		for range 32 {
			raw = append(raw, c)
		}
	}
	if a.DocumentHash() != FileHash(raw) {
		t.Fatal("entry list hash is not over the sorted binary hashes")
	}
}

func TestRootHash(t *testing.T) {
	raw := []byte("4\n0:.:1:920\n" + h3 + ":0:d1:2:920\n")
	ix := mustParse(t, string(raw))
	if ix.RootHash(raw) != FileHash(raw) {
		t.Fatal("schema 4 root hash is not the SHA-256 of the bytes")
	}
	raw3 := []byte("3\n" + h3 + ":80000000:d1:2:920\n")
	ix3 := mustParse(t, string(raw3))
	if ix3.RootHash(raw3) != EntryListHash(ix3.Entries) {
		t.Fatal("schema 3 root hash is not the entry list hash")
	}
	if !ix3.MatchesRoot(raw3, FileHash(raw3)) || !ix3.MatchesRoot(raw3, ix3.RootHash(raw3)) || ix3.MatchesRoot(raw3, h1) {
		t.Fatal("MatchesRoot")
	}
}

func TestWriteRootSchema4(t *testing.T) {
	got := string(WriteRoot(4, []Entry{
		{Hash: h2, Name: "b", Subfiles: 3, Size: 10},
		{Hash: h1, Name: "a", Subfiles: 2, Size: 5},
	}))
	want := "4\n0:.:2:15\n" + h1 + ":0:a:2:5\n" + h2 + ":0:b:3:10\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	got3 := string(WriteRoot(3, []Entry{{Hash: h1, Name: "a", Subfiles: 2, Size: 5}}))
	if got3 != "3\n"+h1+":80000000:a:2:5\n" {
		t.Fatal(got3)
	}
}

func TestRoundTrip(t *testing.T) {
	files := []Entry{
		{Hash: h2, Name: "d.pdf", Size: 9},
		{Hash: h1, Name: "d.metadata", Size: 7},
	}
	doc := WriteDocument(files)
	ix := mustParse(t, string(doc))
	if ix.Schema != 3 || len(ix.Entries) != 2 || ix.Entries[0].Name != "d.metadata" || ix.Entries[0].Type != TypeFile {
		t.Fatalf("%+v", ix)
	}
	if ix.DocumentHash() != EntryListHash(files) {
		t.Fatal("document hash changed")
	}
	again := mustParse(t, string(WriteDocument(ix.Entries)))
	if again.DocumentHash() != ix.DocumentHash() || len(again.Entries) != 2 {
		t.Fatal("round trip changed the document")
	}

	for _, schema := range []int{3, 4} {
		raw := WriteRoot(schema, []Entry{{Hash: ix.DocumentHash(), Name: "d", Subfiles: 2, Size: 16}})
		root := mustParse(t, string(raw))
		raw2 := WriteRoot(root.Schema, root.Entries)
		if string(raw2) != string(raw) || mustParse(t, string(raw2)).RootHash(raw2) != root.RootHash(raw) {
			t.Fatalf("schema %d: round trip changed the root", schema)
		}
	}
}

// TestRmapiData checks indexes that ddvk/rmapi 0.0.35 wrote, with the
// hashes that rmapi computed.
func TestRmapiData(t *testing.T) {
	root, err := os.ReadFile("testdata/rmapi-root-v4.idx")
	if err != nil {
		t.Fatal(err)
	}
	ix := mustParse(t, string(root))
	if ix.Schema != 4 || len(ix.Entries) != 2 {
		t.Fatalf("%+v", ix)
	}
	if got := ix.RootHash(root); got != "2b565e7a2545d6eaf014cb1817df55fde88e56ff116a984591ab35971faba744" {
		t.Fatalf("root hash %s", got)
	}
	if string(WriteRoot(4, ix.Entries)) != string(root) {
		t.Fatal("serializer output differs from rmapi")
	}

	doc, err := os.ReadFile("testdata/rmapi-doc-v3.idx")
	if err != nil {
		t.Fatal(err)
	}
	dx := mustParse(t, string(doc))
	if dx.Schema != 3 || len(dx.Entries) != 4 {
		t.Fatalf("%+v", dx)
	}
	if got := dx.DocumentHash(); got != "826275b78ec1f23317b1a4a0ed7766908fc1695021309c8162fad133395bf947" || got != ix.Entries[0].Hash {
		t.Fatalf("document hash %s", got)
	}
	if string(WriteDocument(dx.Entries)) != string(doc) {
		t.Fatal("document serializer output differs from rmapi")
	}
}

func TestValidHash(t *testing.T) {
	if !ValidHash(h1) || ValidHash(h1[:63]) || ValidHash(strings.Repeat("g", 64)) {
		t.Fatal("ValidHash")
	}
}
