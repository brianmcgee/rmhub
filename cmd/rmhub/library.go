package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// entryView is an entry of the library, as the API gives it.
type entryView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Parent    string     `json:"parent"`
	Trashed   bool       `json:"trashed"`
	Modified  *time.Time `json:"modified"`
	Hash      string     `json:"hash"`
	PageCount int        `json:"pageCount"`
	Path      string     `json:"path,omitempty"`
}

// entryLong is the help text about entry arguments.
const entryLong = `<entry> is an ID or a path. An argument that starts with / is a path,
such as /Work/Paper. The client resolves a path to an ID through the
server. If more than one entry has the path, the command exits with code
5 and prints the IDs. Each other argument is an ID.`

// maxBackoff is the maximum wait between two connection attempts of watch.
const maxBackoff = 30 * time.Second

// resolve returns the entry at a path.
func (cl *client) resolve(ctx context.Context, path string) (entryView, error) {
	var e entryView
	_, err := cl.do(ctx, "GET", "/api/v1/library/resolve?path="+url.QueryEscape(path), nil, &e)
	return e, err
}

// entryID returns the ID of an entry argument. The path / is the top
// level, with the ID "".
func (cl *client) entryID(ctx context.Context, arg string) (string, error) {
	if !strings.HasPrefix(arg, "/") {
		if arg == "" {
			return "", usageErr("an entry argument must not be empty")
		}
		return arg, nil
	}
	if strings.Trim(arg, "/") == "" {
		return "", nil
	}
	e, err := cl.resolve(ctx, arg)
	return e.ID, err
}

// stat returns the entry of an entry argument, with its path.
func (cl *client) stat(ctx context.Context, arg string) (entryView, error) {
	if strings.HasPrefix(arg, "/") {
		return cl.resolve(ctx, arg)
	}
	var e entryView
	_, err := cl.do(ctx, "GET", "/api/v1/library/"+url.PathEscape(arg), nil, &e)
	return e, err
}

// send sends a request with a raw body and headers, and returns the
// response for a status below 400.
func (cl *client) send(ctx context.Context, hc *http.Client, method, path string, body io.Reader, hdr map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, cl.s.URL+path, body)
	if err != nil {
		return nil, usageErr("server URL %q: %v", cl.s.URL, err)
	}
	req.Header.Set("Authorization", "Bearer "+cl.s.Token)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return nil, parseAPIError(res, b)
	}
	return res, nil
}

func (c *cli) entryText(e entryView) func(io.Writer) {
	return func(w io.Writer) {
		mod := "-"
		if e.Modified != nil {
			mod = e.Modified.UTC().Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(w, "ID:\t%s\nName:\t%s\nType:\t%s\nPath:\t%s\nParent:\t%s\nTrashed:\t%t\nModified:\t%s\nPages:\t%d\nHash:\t%s\n",
			e.ID, e.Name, e.Type, e.Path, e.Parent, e.Trashed, mod, e.PageCount, e.Hash)
	}
}

func (c *cli) entriesText(es []entryView) func(io.Writer) {
	return func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "ID\tTYPE\tMODIFIED\tNAME")
		for _, e := range es {
			mod := "-"
			if e.Modified != nil {
				mod = e.Modified.UTC().Format("2006-01-02 15:04")
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.ID, e.Type, mod, e.Name)
		}
	}
}

func (c *cli) libraryCmds() []*cobra.Command {
	return []*cobra.Command{
		c.lsCmd(), c.statCmd(), c.findCmd(), c.getCmd(), c.uploadCmd(),
		c.mkdirCmd(), c.mvCmd(), c.renameCmd(), c.trashCmd(), c.watchCmd(),
		c.pagesCmd(), c.renderCmd(),
	}
}

const entryJSON = `{"id":"…","name":"…","type":"folder|pdf|epub|notebook","parent":"…",
"trashed":false,"modified":"…","hash":"…","pageCount":0,"path":"/…"}`

func (c *cli) lsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls [entry]",
		Short: "List a folder of the library",
		Long: `List the entries of a folder, sorted by name and then by ID. Without
<entry>, list the top level. For a scoped token, the top level is the
list of its grant folders. The ID trash lists the trash.

` + entryLong + `

JSON output: [` + entryJSON + `, …] (without path)`,
		Example: "  rmhub ls\n  rmhub ls /Work\n  rmhub ls trash --json",
		Args:    wrapArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			path := "/api/v1/library"
			if len(args) == 1 {
				id, err := cl.entryID(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if id != "" {
					path += "?parent=" + url.QueryEscape(id)
				}
			}
			var es []entryView
			if _, err := cl.do(cmd.Context(), "GET", path, nil, &es); err != nil {
				return err
			}
			return c.output(es, c.entriesText(es))
		},
	}
}

func (c *cli) statCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stat <entry>",
		Short: "Show an entry of the library",
		Long: `Show the fields of an entry and its path. The hash changes with each
change of the entry. Give it to --if-match of mv, rename and trash.

` + entryLong + `

JSON output: ` + entryJSON,
		Example: "  rmhub stat /Work/Paper\n  rmhub stat 0b7c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3 --json",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			e, err := cl.stat(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return c.output(e, c.entryText(e))
		},
	}
}

func (c *cli) findCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "find <text>",
		Short: "Find entries by name",
		Long: `Find the entries whose name contains <text>, without regard to case. The
result has no trashed entries and at most 100 entries.

JSON output: [` + entryJSON + `, …] (without path)`,
		Example: "  rmhub find invoice",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			var es []entryView
			if _, err := cl.do(cmd.Context(), "GET", "/api/v1/library/search?q="+url.QueryEscape(args[0]), nil, &es); err != nil {
				return err
			}
			return c.output(es, c.entriesText(es))
		},
	}
}

// fileName returns a file name for an entry name. A / in the name
// becomes _.
func fileName(name, ext string) string {
	name = strings.ReplaceAll(name, "/", "_")
	if name == "" || name == "." || name == ".." {
		name = "document"
	}
	return name + ext
}

func (c *cli) getCmd() *cobra.Command {
	var out string
	var archive bool
	cmd := &cobra.Command{
		Use:   "get <entry> [-o <file>] [--archive]",
		Short: "Download a document",
		Long: `Download the source file of a PDF or EPUB document. With --archive,
download a ZIP file with each file of the document, also for a notebook.

The command writes to -o <file>, or to the entry name with the extension
.pdf, .epub or .zip in the current directory. With -o -, it writes to
standard output. The command never replaces a file: if the file exists,
it exits with code 1.

` + entryLong + `

JSON output: {"id":"…","file":"…","size":0}`,
		Example: "  rmhub get /Work/Paper\n  rmhub get /Sketches/Idea --archive -o idea.zip\n  rmhub get /Work/Paper -o - > paper.pdf",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			e, err := cl.stat(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			kind, ext := "content", "."+e.Type
			if archive {
				kind, ext = "archive", ".zip"
			}
			if out == "" {
				out = fileName(e.Name, ext)
			}
			if out != "-" {
				if _, err := os.Lstat(out); err == nil {
					return fmt.Errorf("the file %s exists; the command does not replace it", out)
				}
			}
			res, err := cl.send(cmd.Context(), &http.Client{}, "GET", "/api/v1/library/"+url.PathEscape(e.ID)+"/"+kind, nil, nil)
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
			return c.output(map[string]any{"id": e.ID, "file": out, "size": n}, func(w io.Writer) {
				_, _ = fmt.Fprintln(w, out)
			})
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "output file, or - for standard output")
	cmd.Flags().BoolVar(&archive, "archive", false, "download a ZIP file with each file of the document")
	return cmd
}

// writeNew writes r to a temporary file next to path, then links it to
// path. The link fails if path exists, so the command never replaces a
// file, also when a file appears during the download.
func writeNew(path string, r io.Reader) (int64, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rmhub-get-*")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	n, err := io.Copy(tmp, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	if err := os.Link(tmp.Name(), path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return 0, fmt.Errorf("the file %s exists; the command does not replace it", path)
		}
		return 0, err
	}
	return n, nil
}

func (c *cli) uploadCmd() *cobra.Command {
	var to, name string
	cmd := &cobra.Command{
		Use:   "upload <file> [--to <entry>] [--name <name>]",
		Short: "Upload a PDF or EPUB document",
		Long: `Upload a file with the extension .pdf or .epub as a new document. Other
extensions give exit code 2. The document goes into the folder --to, or
to the top level. The default name is the file name without the
extension.

` + entryLong + `

JSON output: ` + entryJSON,
		Example: "  rmhub upload paper.pdf --to /Inbox\n  rmhub upload book.epub --name \"A Book\"",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file := args[0]
			ext := strings.ToLower(filepath.Ext(file))
			media := map[string]string{".pdf": "application/pdf", ".epub": "application/epub+zip"}[ext]
			if media == "" {
				return usageErr("%s: the file must have the extension .pdf or .epub", file)
			}
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			}
			cl, err := c.client()
			if err != nil {
				return err
			}
			parent := ""
			if to != "" {
				if parent, err = cl.entryID(cmd.Context(), to); err != nil {
					return err
				}
			}
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			q := url.Values{"name": {name}}
			if parent != "" {
				q.Set("parent", parent)
			}
			res, err := cl.send(cmd.Context(), &http.Client{}, "POST", "/api/v1/library/documents?"+q.Encode(), f, map[string]string{"Content-Type": media})
			if err != nil {
				return err
			}
			defer func() { _ = res.Body.Close() }()
			var e entryView
			if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
				return fmt.Errorf("server answer is not valid JSON: %w", err)
			}
			return c.output(e, c.entryText(e))
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "folder for the document (default: the top level)")
	cmd.Flags().StringVar(&name, "name", "", "name of the document (default: the file name without the extension)")
	return cmd
}

func (c *cli) mkdirCmd() *cobra.Command {
	var in string
	cmd := &cobra.Command{
		Use:   "mkdir <name> [--in <entry>]",
		Short: "Create a folder",
		Long: `Create a folder with the name <name> in the folder --in, or at the top
level. Two entries in one folder can have the same name.

` + entryLong + `

JSON output: ` + entryJSON,
		Example: "  rmhub mkdir Inbox\n  rmhub mkdir 2026 --in /Archive",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			parent := ""
			if in != "" {
				if parent, err = cl.entryID(cmd.Context(), in); err != nil {
					return err
				}
			}
			var e entryView
			if _, err := cl.do(cmd.Context(), "POST", "/api/v1/library/folders", map[string]string{"parent": parent, "name": args[0]}, &e); err != nil {
				return err
			}
			return c.output(e, c.entryText(e))
		},
	}
	cmd.Flags().StringVar(&in, "in", "", "parent folder (default: the top level)")
	return cmd
}

// write sends a write request with an optional If-Match header.
func (c *cli) write(ctx context.Context, cl *client, method, path string, body any, ifMatch string) error {
	var rd io.Reader
	hdr := map[string]string{}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = strings.NewReader(string(b))
		hdr["Content-Type"] = "application/json"
	}
	if ifMatch != "" {
		hdr["If-Match"] = `"` + strings.Trim(ifMatch, `"`) + `"`
	}
	res, err := cl.send(ctx, cl.http, method, path, rd, hdr)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	var e entryView
	if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
		return fmt.Errorf("server answer is not valid JSON: %w", err)
	}
	return c.output(e, c.entryText(e))
}

const ifMatchHelp = "do the change only if the entry has this hash (from stat); else exit with code 5"

func (c *cli) mvCmd() *cobra.Command {
	var ifMatch string
	cmd := &cobra.Command{
		Use:   "mv <entry> <folder> [--if-match <hash>]",
		Short: "Move an entry into a folder",
		Long: `Move the entry into the folder. The folder / is the top level. A folder
cannot go into itself or into its descendant. Use trash to move an entry
into the trash.

` + entryLong + `

JSON output: ` + entryJSON,
		Example: "  rmhub mv /Inbox/Paper /Work\n  rmhub mv 0b7c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3 / --if-match 5f2a…",
		Args:    exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			id, err := cl.entryID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if id == "" {
				return usageErr("cannot move the top level")
			}
			parent, err := cl.entryID(cmd.Context(), args[1])
			if err != nil {
				return err
			}
			return c.write(cmd.Context(), cl, "PATCH", "/api/v1/library/"+url.PathEscape(id), map[string]string{"parent": parent}, ifMatch)
		},
	}
	cmd.Flags().StringVar(&ifMatch, "if-match", "", ifMatchHelp)
	return cmd
}

func (c *cli) renameCmd() *cobra.Command {
	var ifMatch string
	cmd := &cobra.Command{
		Use:   "rename <entry> <name> [--if-match <hash>]",
		Short: "Rename an entry",
		Long: `Give the entry the name <name>.

` + entryLong + `

JSON output: ` + entryJSON,
		Example: "  rmhub rename /Inbox/scan Invoice\n  rmhub rename 0b7c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3 Notes --if-match 5f2a…",
		Args:    exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			id, err := cl.entryID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if id == "" {
				return usageErr("cannot rename the top level")
			}
			return c.write(cmd.Context(), cl, "PATCH", "/api/v1/library/"+url.PathEscape(id), map[string]string{"name": args[1]}, ifMatch)
		},
	}
	cmd.Flags().StringVar(&ifMatch, "if-match", "", ifMatchHelp)
	return cmd
}

func (c *cli) trashCmd() *cobra.Command {
	var ifMatch string
	cmd := &cobra.Command{
		Use:   "trash <entry> [--if-match <hash>]",
		Short: "Move an entry into the trash",
		Long: `Move the entry into the trash. The owner can restore it on the tablet.
The API cannot remove an entry from the library.

` + entryLong + `

JSON output: ` + entryJSON,
		Example: "  rmhub trash /Inbox/old-scan",
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
			if id == "" {
				return usageErr("cannot trash the top level")
			}
			return c.write(cmd.Context(), cl, "POST", "/api/v1/library/"+url.PathEscape(id)+"/trash", nil, ifMatch)
		},
	}
	cmd.Flags().StringVar(&ifMatch, "if-match", "", ifMatchHelp)
	return cmd
}

func (c *cli) watchCmd() *cobra.Command {
	var after string
	cmd := &cobra.Command{
		Use:   "watch [--after <generation>]",
		Short: "Print the changes of the library",
		Long: `Read the change feed and print each event as one line of JSON. Without
--after, the feed starts after the current generation. After a lost
connection, the command connects again and continues after the last
printed generation, so no event is lost. It waits at most 30 seconds
between two attempts. Stop it with Ctrl-C.

JSON output, one line for each event, also without --json:
{"generation":0,"source":{"type":"device|token","id":"…"},
"changes":[{"id":"…","op":"created|updated|moved|trashed|removed","entry":{…}|null}]}`,
		Example: "  rmhub watch\n  rmhub watch --after 41",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if after != "" {
				if _, err := strconv.ParseUint(after, 10, 64); err != nil {
					return usageErr("--after %q: give a generation number", after)
				}
			}
			cl, err := c.client()
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return c.watch(ctx, cl, after)
		},
	}
	cmd.Flags().StringVar(&after, "after", "", "start after this generation")
	return cmd
}

// watch reads the feed until ctx ends. It connects again after a lost
// connection or a server error.
func (c *cli) watch(ctx context.Context, cl *client, last string) error {
	backoff := time.Second
	hc := &http.Client{}
	for {
		hdr := map[string]string{"Accept": "text/event-stream"}
		if last != "" {
			hdr["Last-Event-ID"] = last
		}
		res, err := cl.send(ctx, hc, "GET", "/api/v1/changes", nil, hdr)
		if ctx.Err() != nil {
			return nil
		}
		var ae *apiError
		if errors.As(err, &ae) && ae.status < 500 {
			return err
		}
		if err == nil {
			backoff = time.Second
			last, err = c.readFeed(res.Body, last)
			_ = res.Body.Close()
			if ctx.Err() != nil {
				return nil
			}
		}
		if err != nil {
			_, _ = fmt.Fprintf(c.stderr, "rmhub: change feed: %v; connecting again in %s\n", err, backoff)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(2*backoff, maxBackoff)
	}
}

// readFeed prints the data of each event and returns the ID of the last
// event.
func (c *cli) readFeed(body io.Reader, last string) (string, error) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var id, data string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "id:"):
			id = strings.TrimSpace(line[3:])
		case strings.HasPrefix(line, "data:"):
			data = strings.TrimSpace(line[5:])
		case line == "":
			if data != "" {
				if _, err := fmt.Fprintln(c.stdout, data); err != nil {
					return last, err
				}
				if id != "" {
					last = id
				}
			}
			id, data = "", ""
		}
	}
	if err := sc.Err(); err != nil {
		return last, err
	}
	return last, errors.New("the server ended the stream")
}
