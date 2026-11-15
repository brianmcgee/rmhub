package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

// pageView is a page of a document, as the API gives it.
type pageView struct {
	Number  int     `json:"number"`
	ID      string  `json:"id"`
	PDFPage *int    `json:"pdfPage"`
	RMHash  *string `json:"rmHash"`
}

func (c *cli) pagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pages <entry>",
		Short: "List the pages of a document",
		Long: `List the pages of a document in the order of the tablet. The number of a
page is the value for render --page. PDF is the page of the PDF under the
page, or - for a page without PDF. STROKES is yes if the page has a .rm
file. A PDF that no tablet opened yet has one page for each PDF page,
with no ID (-).

` + entryLong + `

JSON output: [{"number":1,"id":"…","pdfPage":1|null,"rmHash":"…"|null}, …]`,
		Example: "  rmhub pages /Notes\n  rmhub pages /Work/Paper --json",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			id, err := cl.entryID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			var v struct {
				Pages []pageView `json:"pages"`
			}
			if _, err := cl.do(cmd.Context(), "GET", "/api/v1/library/"+url.PathEscape(id)+"/pages", nil, &v); err != nil {
				return err
			}
			return c.output(v.Pages, func(w io.Writer) {
				_, _ = fmt.Fprintln(w, "NUMBER\tID\tPDF\tSTROKES")
				for _, p := range v.Pages {
					id, pdf, strokes := p.ID, "-", "no"
					if id == "" {
						id = "-"
					}
					if p.PDFPage != nil {
						pdf = strconv.Itoa(*p.PDFPage)
					}
					if p.RMHash != nil {
						strokes = "yes"
					}
					_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", p.Number, id, pdf, strokes)
				}
			})
		},
	}
}

func (c *cli) renderCmd() *cobra.Command {
	var out, format string
	var page, width int
	cmd := &cobra.Command{
		Use:   "render <entry> --page <number> [--format png|svg] [--width <px>] [-o <file>]",
		Short: "Render a page as an image",
		Long: `Render a page of a notebook or a PDF document as a PNG or SVG image. The
image shows the handwriting, and on a PDF page also the PDF. A PDF page
is available only as PNG. rmhub does not render EPUB pages. The width
is from 100 to 4096 pixels. Get the page numbers with rmhub pages.

The command writes to -o <file>, or to <name>-p<number>.<format> in the
current directory. With -o -, it writes to standard output. The command
never replaces a file: if the file exists, it exits with code 1.

` + entryLong + `

JSON output: {"id":"…","page":1,"file":"…","size":0}`,
		Example: "  rmhub render /Notes --page 3 -o page.png\n  rmhub render /Notes --page 1 --format svg\n  rmhub render /Work/Paper --page 2 --width 702 -o - > p2.png",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if page < 1 {
				return usageErr("--page must be a page number from 1")
			}
			if format != "png" && format != "svg" {
				return usageErr("--format must be png or svg")
			}
			cl, err := c.client()
			if err != nil {
				return err
			}
			e, err := cl.stat(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if out == "" {
				out = fileName(fmt.Sprintf("%s-p%d", e.Name, page), "."+format)
			}
			if out != "-" {
				if _, err := os.Lstat(out); err == nil {
					return fmt.Errorf("the file %s exists; the command does not replace it", out)
				}
			}
			q := url.Values{"format": {format}, "width": {strconv.Itoa(width)}}
			path := fmt.Sprintf("/api/v1/library/%s/pages/%d/render?%s", url.PathEscape(e.ID), page, q.Encode())
			res, err := cl.send(cmd.Context(), &http.Client{}, "GET", path, nil, nil)
			if err != nil {
				return err
			}
			defer func() { _ = res.Body.Close() }()
			if out == "-" {
				_, err := io.Copy(c.stdout, res.Body)
				return err
			}
			n, err := writeNew(out, res.Body)
			if err != nil {
				return err
			}
			return c.output(map[string]any{"id": e.ID, "page": page, "file": out, "size": n}, func(w io.Writer) {
				_, _ = fmt.Fprintln(w, out)
			})
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number, from 1 (required)")
	cmd.Flags().StringVar(&format, "format", "png", "image format: png or svg")
	cmd.Flags().IntVar(&width, "width", 1404, "image width in pixels, from 100 to 4096")
	cmd.Flags().StringVarP(&out, "output", "o", "", "output file, or - for standard output")
	return cmd
}
