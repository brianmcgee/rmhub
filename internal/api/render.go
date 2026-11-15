package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/singleflight"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/ratelimit"
	"github.com/brianmcgee/rmhub/internal/render"
	"github.com/brianmcgee/rmhub/internal/rendercache"
)

// Render width limits in pixels.
const (
	MinRenderWidth     = 100
	MaxRenderWidth     = 4096
	DefaultRenderWidth = int(render.PageWidth)
)

// Renderer renders pages for the render endpoint. It keeps the results in
// a cache, runs one render for parallel requests of one image, and limits
// the number of renders at the same time.
type Renderer struct {
	Cache *rendercache.Cache
	Log   *slog.Logger
	// Limit limits the renders of each account. Nil means no limit.
	Limit *ratelimit.Limiter
	// Renders is the number of renders since the start.
	Renders atomic.Int64

	sem   chan struct{}
	group singleflight.Group

	pdfOnce sync.Once
	pdf     *render.PDF
	pdfErr  error
}

// NewRenderer returns a renderer with at most n renders at the same time.
func NewRenderer(cache *rendercache.Cache, n int, log *slog.Logger) *Renderer {
	return &Renderer{Cache: cache, Log: log, sem: make(chan struct{}, n)}
}

// Workers returns the maximum number of renders at the same time.
func (r *Renderer) Workers() int { return cap(r.sem) }

// pdfPool starts the PDFium pool at the first PDF render, so that a server
// without PDF renders does not pay for it.
func (r *Renderer) pdfPool() (*render.PDF, error) {
	r.pdfOnce.Do(func() { r.pdf, r.pdfErr = render.NewPDF(context.Background(), cap(r.sem)) })
	return r.pdf, r.pdfErr
}

// Close stops the PDFium pool if it runs.
func (r *Renderer) Close() error {
	r.pdfOnce.Do(func() { r.pdfErr = errors.New("the renderer is stopped") })
	if r.pdf != nil {
		return r.pdf.Close()
	}
	return nil
}

// image returns the cached result with the key, or runs do and keeps its
// result. Parallel calls with one key run do once. Each run of do takes
// one unit from the render limit of the account; a cached result takes
// none.
func (r *Renderer) image(ctx context.Context, account, key string, do func(context.Context) ([]byte, error)) ([]byte, error) {
	if b, ok := r.Cache.Get(key); ok {
		return b, nil
	}
	v, err, _ := r.group.Do(key, func() (any, error) {
		if b, ok := r.Cache.Get(key); ok {
			return b, nil
		}
		if err := take(r.Limit, account, "page renders of this account"); err != nil {
			return nil, err
		}
		// The result goes to each caller, so the first caller must not
		// cancel it.
		ctx := context.WithoutCancel(ctx)
		r.sem <- struct{}{}
		defer func() { <-r.sem }()
		b, err := do(ctx)
		if err != nil {
			return nil, err
		}
		r.Renders.Add(1)
		if err := r.Cache.Put(key, b); err != nil {
			r.Log.Warn("cannot keep a render result", "err", err)
		}
		return b, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

var renderMedia = map[render.Format]string{render.PNG: "image/png", render.SVG: "image/svg+xml"}

func (l *Library) renderPage(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	format := render.Format(q.Get("format"))
	if format == "" {
		format = render.PNG
	}
	if renderMedia[format] == "" {
		return Errorf(CodeBadRequest, "the format %q is not known (use png or svg)", format)
	}
	width := DefaultRenderWidth
	if s := q.Get("width"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < MinRenderWidth || n > MaxRenderWidth {
			return Errorf(CodeBadRequest, "the width must be an integer from %d to %d", MinRenderWidth, MaxRenderWidth)
		}
		width = n
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		return Errorf(CodeNotFound, "the page %q is not a page number", r.PathValue("number"))
	}

	v, err := l.open(w, r)
	if err != nil {
		return err
	}
	d, err := v.doc(r.PathValue("id"))
	if err != nil {
		return err
	}
	if d.Type == library.TypeEPUB {
		return Errorf(CodeNotFound, "entry %s is an EPUB document, and rmhub does not render EPUB pages", d.ID)
	}
	ps, err := l.pageList(r.Context(), v.s.Account, d)
	if err != nil {
		return err
	}
	if number < 1 || number > len(ps) {
		return Errorf(CodeNotFound, "entry %s has %d pages and no page %d", d.ID, len(ps), number)
	}
	p := ps[number-1]

	var rmHash, pdfHash, pdfPage string
	if p.RMHash != nil {
		rmHash = *p.RMHash
	}
	if p.PDFPage != nil && d.Type == library.TypePDF {
		f, ok := d.File(".pdf")
		if !ok {
			return Errorf(CodeNotFound, "entry %s has no .pdf file", d.ID)
		}
		if format != render.PNG {
			return Errorf(CodeBadRequest, "page %d of entry %s is a PDF page, and rmhub renders PDF pages only as png", number, d.ID)
		}
		pdfHash, pdfPage = f.Hash, strconv.Itoa(*p.PDFPage)
	}
	key := rendercache.Key(v.s.Account, strconv.Itoa(render.Version), string(format), strconv.Itoa(width), rmHash, pdfHash, pdfPage)
	etag := `"` + key + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	ns := mustNS(v.s.Account)
	b, err := l.Render.image(r.Context(), v.s.Account, key, func(ctx context.Context) ([]byte, error) {
		var rm []byte
		if rmHash != "" {
			if rm, err = l.blobMax(ctx, ns, rmHash, d.ID, maxPageFile); err != nil {
				return nil, err
			}
		}
		if pdfHash == "" {
			return render.Render(rm, nil, format, width)
		}
		pdf, err := l.blob(ctx, ns, pdfHash, d.ID)
		if err != nil {
			return nil, err
		}
		pool, err := l.Render.pdfPool()
		if err != nil {
			return nil, err
		}
		return pool.Render(ctx, pdf, *p.PDFPage, rm, width)
	})
	if errors.Is(err, render.ErrTooLarge) {
		return Errorf(CodeBadRequest, "page %d of entry %s is too large to render with the width %d; request a smaller width", number, d.ID, width)
	}
	if err != nil {
		return fmt.Errorf("render page %d of document %s: %w", number, d.ID, err)
	}
	w.Header().Set("Content-Type", renderMedia[format])
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
	return nil
}

// maxPageFile is the maximum size in bytes of a .rm file that the
// server reads into memory for a render.
const maxPageFile = 32 << 20

// blob reads a file of the document id from the blob store.
func (l *Library) blob(ctx context.Context, ns blob.Namespace, hash, id string) ([]byte, error) {
	return l.blobMax(ctx, ns, hash, id, math.MaxInt64)
}

// blobMax is blob with a size limit. A larger file gives code conflict.
func (l *Library) blobMax(ctx context.Context, ns blob.Namespace, hash, id string, limit int64) ([]byte, error) {
	rc, size, err := l.Blobs.Get(ctx, ns, hash)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, Errorf(CodeNotFound, "a file of entry %s is not in the blob store", id)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	if size > limit {
		return nil, Errorf(CodeConflict, "a file of entry %s has %d bytes, and the limit is %d bytes", id, size, limit)
	}
	return io.ReadAll(rc)
}
