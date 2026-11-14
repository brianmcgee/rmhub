// Package hashtree parses, hashes and writes the index files of the
// reMarkable Sync 1.5 hash tree. The package does no I/O.
//
// A root index names one document index for each document. A document
// index names the files of the document. Schema 3 and schema 4 are
// supported.
package hashtree

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Entry types.
const (
	// TypeFile is the type of a file entry, and of a root entry in schema 4.
	TypeFile = "0"
	// TypeDoc is the type of a root entry in schema 3.
	TypeDoc = "80000000"
)

// Entry is one line of an index.
type Entry struct {
	Hash     string
	Type     string
	Name     string
	Subfiles int
	Size     int64
}

// Index is a parsed index file.
type Index struct {
	Schema  int
	Entries []Entry
}

// ValidHash tells if s is 64 lowercase hexadecimal characters.
func ValidHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Parse parses an index of schema 3 or schema 4. It ignores empty lines
// and the count and size of the schema 4 summary.
func Parse(b []byte) (*Index, error) {
	lines := strings.Split(string(b), "\n")
	next := 0
	nextLine := func() (string, bool) {
		for next < len(lines) {
			l := strings.TrimSuffix(lines[next], "\r")
			next++
			if l != "" {
				return l, true
			}
		}
		return "", false
	}
	first, ok := nextLine()
	if !ok {
		return nil, errors.New("index is empty")
	}
	ix := &Index{}
	switch first {
	case "3":
		ix.Schema = 3
	case "4":
		ix.Schema = 4
		summary, ok := nextLine()
		if !ok {
			return nil, errors.New("schema 4 index has no summary line")
		}
		if err := parseSummary(summary); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("schema %q is not supported", first)
	}
	for {
		l, ok := nextLine()
		if !ok {
			break
		}
		e, err := parseEntry(l)
		if err != nil {
			return nil, fmt.Errorf("line %d %q: %w", next, l, err)
		}
		ix.Entries = append(ix.Entries, e)
	}
	return ix, nil
}

func parseSummary(l string) error {
	f := strings.Split(l, ":")
	if len(f) != 4 || f[0] != "0" || f[1] != "." {
		return fmt.Errorf("summary line %q is not valid", l)
	}
	if _, err := strconv.Atoi(f[2]); err != nil {
		return fmt.Errorf("summary line %q has no valid count", l)
	}
	if _, err := strconv.ParseInt(f[3], 10, 64); err != nil {
		return fmt.Errorf("summary line %q has no valid size", l)
	}
	return nil
}

func parseEntry(l string) (Entry, error) {
	f := strings.Split(l, ":")
	if len(f) != 5 {
		return Entry{}, fmt.Errorf("entry has %d fields, not 5", len(f))
	}
	if !ValidHash(f[0]) {
		return Entry{}, errors.New("entry hash is not 64 lowercase hexadecimal characters")
	}
	if f[1] == "" || f[2] == "" {
		return Entry{}, errors.New("entry type or name is empty")
	}
	sub, err := strconv.Atoi(f[3])
	if err != nil || sub < 0 {
		return Entry{}, errors.New("entry subfiles value is not a decimal integer")
	}
	size, err := strconv.ParseInt(f[4], 10, 64)
	if err != nil || size < 0 {
		return Entry{}, errors.New("entry size is not a decimal integer")
	}
	return Entry{Hash: f[0], Type: f[1], Name: f[2], Subfiles: sub, Size: size}, nil
}

// FileHash returns the hash of a file: the hex SHA-256 of its bytes.
func FileHash(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// EntryListHash returns the SHA-256 of the binary entry hashes, in
// ascending byte order of the entry names. The entries must have valid
// hashes.
func EntryListHash(entries []Entry) string {
	sorted := slices.Clone(entries)
	sortEntries(sorted)
	h := sha256.New()
	var buf [32]byte
	for _, e := range sorted {
		if _, err := hex.Decode(buf[:], []byte(e.Hash)); err != nil {
			panic("hashtree: entry hash is not valid: " + e.Hash)
		}
		h.Write(buf[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// DocumentHash returns the hash of a document index.
func (ix *Index) DocumentHash() string { return EntryListHash(ix.Entries) }

// RootHash returns the hash of a root index with the bytes raw: the
// entry list hash in schema 3, and the SHA-256 of raw in schema 4.
func (ix *Index) RootHash(raw []byte) string {
	if ix.Schema == 3 {
		return EntryListHash(ix.Entries)
	}
	return FileHash(raw)
}

// MatchesRoot tells if hash names the root index with the bytes raw. A
// schema 3 root also matches the SHA-256 of its bytes, because rmapi
// hashes a schema 3 root in that way.
func (ix *Index) MatchesRoot(raw []byte, hash string) bool {
	return ix.RootHash(raw) == hash || FileHash(raw) == hash
}

func sortEntries(es []Entry) {
	slices.SortStableFunc(es, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
}

// WriteRoot writes a root index of the schema 3 or 4. Each entry names a
// document index with its hash, subfile count and size. WriteRoot sets
// the entry type for the schema.
func WriteRoot(schema int, docs []Entry) []byte {
	es := slices.Clone(docs)
	sortEntries(es)
	typ := TypeFile
	if schema == 3 {
		typ = TypeDoc
	}
	var b bytes.Buffer
	b.WriteString(strconv.Itoa(schema))
	b.WriteByte('\n')
	if schema == 4 {
		var total int64
		for _, e := range es {
			total += e.Size
		}
		fmt.Fprintf(&b, "0:.:%d:%d\n", len(es), total)
	}
	for _, e := range es {
		e.Type = typ
		writeEntry(&b, e)
	}
	return b.Bytes()
}

// WriteDocument writes a schema 3 document index for the files.
func WriteDocument(files []Entry) []byte {
	es := slices.Clone(files)
	sortEntries(es)
	var b bytes.Buffer
	b.WriteString("3\n")
	for _, e := range es {
		e.Type = TypeFile
		e.Subfiles = 0
		writeEntry(&b, e)
	}
	return b.Bytes()
}

func writeEntry(b *bytes.Buffer, e Entry) {
	fmt.Fprintf(b, "%s:%s:%s:%d:%d\n", e.Hash, e.Type, e.Name, e.Subfiles, e.Size)
}
