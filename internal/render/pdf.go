package render

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// PDF renders PDF pages with PDFium in WebAssembly. It needs no cgo.
type PDF struct {
	pool pdfium.Pool
	// timeout is the time limit of the PDFium work of one call.
	timeout time.Duration
}

// pdfTimeout is the time limit of the PDFium work of one call. A PDF
// file can keep PDFium busy without end.
const pdfTimeout = time.Minute

// ErrTimeout is the error for a PDF that PDFium cannot open or draw in
// the time limit.
var ErrTimeout = errors.New("the PDF needs too much time to render")

// NewPDF starts a pool of PDFium instances with at most n instances.
// The instances get no file system.
func NewPDF(ctx context.Context, n int) (*PDF, error) {
	return newPDF(ctx, n, pdfMemoryPages)
}

// pdfMemoryPages is the memory limit of one PDFium instance in
// WebAssembly pages of 64 KiB: 1 GiB. Without a limit, one PDF can make
// an instance use 4 GiB.
const pdfMemoryPages = 1 << 30 >> 16

func newPDF(ctx context.Context, n int, memoryPages uint32) (*PDF, error) {
	pool, err := webassembly.Init(webassembly.Config{
		Context:  ctx,
		MinIdle:  0,
		MaxIdle:  n,
		MaxTotal: n,
		FSConfig: wazero.NewFSConfig(),
		// PDFium needs the exception handling feature, as in the
		// default configuration of go-pdfium.
		// WithCloseOnContextDone lets Kill stop an instance that is busy.
		RuntimeConfig: wazero.NewRuntimeConfig().
			WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
			WithMemoryLimitPages(memoryPages).
			WithCloseOnContextDone(true),
	})
	if err != nil {
		return nil, fmt.Errorf("start PDFium: %w", err)
	}
	return &PDF{pool: pool, timeout: pdfTimeout}, nil
}

// use calls fn with an instance of the pool. It kills the instance when
// fn runs longer than the time limit or when ctx ends. The pool then
// makes a new instance for the next call.
func (p *PDF) use(ctx context.Context, fn func(inst pdfium.Pdfium) error) error {
	inst, err := p.pool.GetInstanceWithContext(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = inst.Kill() })
	err = fn(inst)
	if !stop() {
		// Kill ran or runs, and it removes the instance from the pool.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ErrTimeout
		}
		return ctx.Err()
	}
	_ = inst.Close()
	return err
}

// Close stops the pool.
func (p *PDF) Close() error { return p.pool.Close() }

// PageCount returns the number of pages of the PDF file data.
func (p *PDF) PageCount(ctx context.Context, data []byte) (int, error) {
	var count int
	err := p.use(ctx, func(inst pdfium.Pdfium) error {
		doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
		if err != nil {
			return fmt.Errorf("open the PDF: %w", err)
		}
		defer func() { _, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document}) }()
		n, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
		if err != nil {
			return err
		}
		count = n.PageCount
		return nil
	})
	return count, err
}

// Render draws the page pdfPage (from 1) of the PDF file data with the
// strokes of the .rm file rm (or nil) on top, as PNG. The document stays
// open only during the call.
func (p *PDF) Render(ctx context.Context, data []byte, pdfPage int, rm []byte, width int) ([]byte, error) {
	var out []byte
	err := p.use(ctx, func(inst pdfium.Pdfium) error {
		doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
		if err != nil {
			return fmt.Errorf("open the PDF: %w", err)
		}
		defer func() { _, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document}) }()
		n, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
		if err != nil {
			return err
		}
		if pdfPage < 1 || pdfPage > n.PageCount {
			return fmt.Errorf("the PDF has %d pages and no page %d", n.PageCount, pdfPage)
		}
		out, err = Render(rm, &pdfBase{inst: inst, doc: doc.Document, index: pdfPage - 1}, PNG, width)
		return err
	})
	return out, err
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
