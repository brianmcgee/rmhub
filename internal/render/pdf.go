package render

import (
	"context"
	"fmt"
	"image"
	"image/draw"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
)

// PDF renders PDF pages with PDFium in WebAssembly. It needs no cgo.
type PDF struct {
	pool pdfium.Pool
}

// NewPDF starts a pool of PDFium instances with at most n instances.
// The instances get no file system.
func NewPDF(ctx context.Context, n int) (*PDF, error) {
	pool, err := webassembly.Init(webassembly.Config{
		Context:  ctx,
		MinIdle:  0,
		MaxIdle:  n,
		MaxTotal: n,
		FSConfig: wazero.NewFSConfig(),
	})
	if err != nil {
		return nil, fmt.Errorf("start PDFium: %w", err)
	}
	return &PDF{pool: pool}, nil
}

// Close stops the pool.
func (p *PDF) Close() error { return p.pool.Close() }

// PageCount returns the number of pages of the PDF file data.
func (p *PDF) PageCount(ctx context.Context, data []byte) (int, error) {
	inst, err := p.pool.GetInstanceWithContext(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = inst.Close() }()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return 0, fmt.Errorf("open the PDF: %w", err)
	}
	defer func() { _, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document}) }()
	n, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return 0, err
	}
	return n.PageCount, nil
}

// Render draws the page pdfPage (from 1) of the PDF file data with the
// strokes of the .rm file rm (or nil) on top, as PNG. The document stays
// open only during the call.
func (p *PDF) Render(ctx context.Context, data []byte, pdfPage int, rm []byte, width int) ([]byte, error) {
	inst, err := p.pool.GetInstanceWithContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = inst.Close() }()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, fmt.Errorf("open the PDF: %w", err)
	}
	defer func() { _, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document}) }()
	n, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, err
	}
	if pdfPage < 1 || pdfPage > n.PageCount {
		return nil, fmt.Errorf("the PDF has %d pages and no page %d", n.PageCount, pdfPage)
	}
	return Render(rm, &pdfBase{inst: inst, doc: doc.Document, index: pdfPage - 1}, PNG, width)
}

// pdfBase is one page of an open document.
type pdfBase struct {
	inst  pdfium.Pdfium
	doc   references.FPDF_DOCUMENT
	index int
}

func (b *pdfBase) page() requests.Page {
	return requests.Page{ByIndex: &requests.PageByIndex{Document: b.doc, Index: b.index}}
}

func (b *pdfBase) Size() (float64, float64, error) {
	s, err := b.inst.GetPageSize(&requests.GetPageSize{Page: b.page()})
	if err != nil {
		return 0, 0, err
	}
	return s.Width, s.Height, nil
}

func (b *pdfBase) Render(px int) (image.Image, error) {
	r, err := b.inst.RenderPageInPixels(&requests.RenderPageInPixels{Page: b.page(), Width: px})
	if err != nil {
		return nil, fmt.Errorf("render the PDF page: %w", err)
	}
	defer r.Cleanup()
	// The pixel buffer is valid only until Cleanup, so copy it.
	src := r.Result.RenderedImage
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	return img, nil
}
