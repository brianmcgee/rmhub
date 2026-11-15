package library

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/hashtree"
)

// PutFile writes a file blob and returns its index entry with the name
// id+ext.
func PutFile(ctx context.Context, blobs blob.Store, ns blob.Namespace, id, ext string, body []byte) (hashtree.Entry, error) {
	h := hashtree.FileHash(body)
	if err := blobs.Put(ctx, ns, h, bytes.NewReader(body)); err != nil {
		return hashtree.Entry{}, err
	}
	return hashtree.Entry{Hash: h, Type: hashtree.TypeFile, Name: id + ext, Size: int64(len(body))}, nil
}

// PutJSON writes v as a JSON file blob.
func PutJSON(ctx context.Context, blobs blob.Store, ns blob.Namespace, id, ext string, v any) (hashtree.Entry, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return hashtree.Entry{}, err
	}
	return PutFile(ctx, blobs, ns, id, ext, b)
}

// PutDocument writes the document index of the files and returns the
// root entry of the document.
func PutDocument(ctx context.Context, blobs blob.Store, ns blob.Namespace, id string, files []hashtree.Entry) (hashtree.Entry, error) {
	idx := hashtree.WriteDocument(files)
	h := hashtree.EntryListHash(files)
	if err := blobs.Put(ctx, ns, h, bytes.NewReader(idx)); err != nil {
		return hashtree.Entry{}, err
	}
	var size int64
	for _, f := range files {
		size += f.Size
	}
	return hashtree.Entry{Hash: h, Type: hashtree.TypeDoc, Name: id, Subfiles: len(files), Size: size}, nil
}

// PutRoot writes a root index of the schema and returns its hash.
func PutRoot(ctx context.Context, blobs blob.Store, ns blob.Namespace, schema int, docs []hashtree.Entry) (string, error) {
	raw := hashtree.WriteRoot(schema, docs)
	ix, err := hashtree.Parse(raw)
	if err != nil {
		return "", err
	}
	h := ix.RootHash(raw)
	if err := blobs.Put(ctx, ns, h, bytes.NewReader(raw)); err != nil {
		return "", err
	}
	return h, nil
}

// ReplaceFile returns a copy of files with the entry of the same name
// replaced by f, or with f added.
func ReplaceFile(files []hashtree.Entry, f hashtree.Entry) []hashtree.Entry {
	out := slices.Clone(files)
	for i := range out {
		if out[i].Name == f.Name {
			out[i] = f
			return out
		}
	}
	return append(out, f)
}
