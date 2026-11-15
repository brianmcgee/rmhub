// Package pages reads the ordered page list of a document from its
// .content file.
package pages

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/brianmcgee/rmhub/internal/hashtree"
)

// Page is one page of a document.
type Page struct {
	// Number is the position of the page in the document, from 1.
	Number int    `json:"number"`
	ID     string `json:"id"`
	// PDFPage is the page of the PDF under the page, from 1, or nil.
	PDFPage *int `json:"pdfPage"`
	// RMHash is the hash of the .rm file of the page, or nil if the page
	// has no strokes.
	RMHash *string `json:"rmHash"`
}

type value[T any] struct {
	Value T `json:"value"`
}

type cPage struct {
	ID      string        `json:"id"`
	Idx     value[string] `json:"idx"`
	Redir   *value[int]   `json:"redir"`
	Deleted *value[int]   `json:"deleted"`
}

type content struct {
	FileType string `json:"fileType"`
	CPages   *struct {
		Pages []cPage `json:"pages"`
	} `json:"cPages"`
	Pages       []string `json:"pages"`
	Redirection []int    `json:"redirectionPageMap"`
}

// List returns the pages of the document docID. raw is the .content
// file; nil gives no pages. files is the document index, which gives the
// .rm file of each page.
func List(docID string, raw []byte, files []hashtree.Entry) ([]Page, error) {
	if raw == nil {
		return []Page{}, nil
	}
	var c content
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("the .content file of document %s is not valid: %w", docID, err)
	}
	rm := make(map[string]string, len(files))
	for _, f := range files {
		rm[f.Name] = f.Hash
	}
	out := []Page{}
	add := func(id string, pdf int) {
		p := Page{Number: len(out) + 1, ID: id}
		if pdf >= 0 {
			p.PDFPage = new(pdf + 1)
		}
		if h, ok := rm[docID+"/"+id+".rm"]; ok {
			p.RMHash = new(h)
		}
		out = append(out, p)
	}
	if c.CPages != nil && c.CPages.Pages != nil {
		ps := make([]cPage, 0, len(c.CPages.Pages))
		for _, p := range c.CPages.Pages {
			if p.Deleted == nil || p.Deleted.Value == 0 {
				ps = append(ps, p)
			}
		}
		sort.SliceStable(ps, func(i, j int) bool {
			if ps[i].Idx.Value != ps[j].Idx.Value {
				return ps[i].Idx.Value < ps[j].Idx.Value
			}
			return ps[i].ID < ps[j].ID
		})
		for _, p := range ps {
			pdf := -1
			if p.Redir != nil {
				pdf = p.Redir.Value
			}
			add(p.ID, pdf)
		}
		return out, nil
	}
	for i, id := range c.Pages {
		pdf := -1
		if c.FileType == "pdf" && i < len(c.Redirection) {
			pdf = c.Redirection[i]
		}
		add(id, pdf)
	}
	return out, nil
}
