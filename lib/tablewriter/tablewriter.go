package tablewriter

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/acarl005/stripansi"
)

type Column struct {
	Name         string
	SeparateLine bool
	Lines        int
	RightAlign   bool
}

type tableCfg struct {
	borders bool
}

type TableOption func(*tableCfg)

func WithBorders() TableOption {
	return func(c *tableCfg) {
		c.borders = true
	}
}

type columnCfg struct {
	rightAlign bool
}

type ColumnOption func(*columnCfg)

func RightAlign() ColumnOption {
	return func(c *columnCfg) {
		c.rightAlign = true
	}
}

type TableWriter struct {
	cols []Column
	rows []map[int]string
}

func Col(name string, opts ...ColumnOption) Column {
	cfg := &columnCfg{}
	for _, o := range opts {
		o(cfg)
	}
	return Column{
		Name:         name,
		SeparateLine: false,
		RightAlign:   cfg.rightAlign,
	}
}

func NewLineCol(name string) Column {
	return Column{
		Name:         name,
		SeparateLine: true,
	}
}

// New creates a new TableWriter. Unlike text/tabwriter, this works with CLI escape codes, and
// allows for info
//
//	in separate lines
func New(cols ...Column) *TableWriter {
	return &TableWriter{
		cols: cols,
	}
}

func (w *TableWriter) Write(r map[string]interface{}) {
	// this can cause columns to be out of order, but will at least work
	byColID := map[int]string{}

cloop:
	for col, val := range r {
		for i, column := range w.cols {
			if column.Name == col {
				byColID[i] = fmt.Sprint(val)
				w.cols[i].Lines++
				continue cloop
			}
		}

		byColID[len(w.cols)] = fmt.Sprint(val)
		w.cols = append(w.cols, Column{
			Name:         col,
			SeparateLine: false,
			Lines:        1,
		})
	}

	w.rows = append(w.rows, byColID)
}

func (w *TableWriter) Flush(out io.Writer, opts ...TableOption) error {
	cfg := &tableCfg{}
	for _, o := range opts {
		o(cfg)
	}

	colLengths := make([]int, len(w.cols))

	header := map[int]string{}
	for i, col := range w.cols {
		if col.SeparateLine {
			continue
		}
		header[i] = col.Name
	}

	w.rows = append([]map[int]string{header}, w.rows...)

	for col, c := range w.cols {
		if c.Lines == 0 {
			continue
		}

		for _, row := range w.rows {
			val, found := row[col]
			if !found {
				continue
			}

			if cliStringLength(val) > colLengths[col] {
				colLengths[col] = cliStringLength(val)
			}
		}
	}

	if cfg.borders {
		// top line
		if _, err := fmt.Fprint(out, "┌"); err != nil {
			return err
		}
		for ci, col := range w.cols {
			if col.Lines == 0 {
				continue
			}
			if _, err := fmt.Fprint(out, strings.Repeat("─", colLengths[ci]+2)); err != nil {
				return err
			}
			if ci != len(w.cols)-1 {
				if _, err := fmt.Fprint(out, "┬"); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintln(out, "┐"); err != nil {
			return err
		}
	}

	for lineNumber, row := range w.rows {
		cols := make([]string, len(w.cols))

		if cfg.borders {
			if _, err := fmt.Fprint(out, "│ "); err != nil {
				return err
			}
		}

		for ci, col := range w.cols {
			if col.Lines == 0 {
				continue
			}

			e := row[ci]
			pad := colLengths[ci] - cliStringLength(e) + 2
			if cfg.borders {
				pad--
			}
			if !col.SeparateLine && col.Lines > 0 {
				if col.RightAlign {
					e = strings.Repeat(" ", pad-1) + e + " "
				} else {
					e = e + strings.Repeat(" ", pad)
				}
				if _, err := fmt.Fprint(out, e); err != nil {
					return err
				}
				if cfg.borders {
					if _, err := fmt.Fprint(out, "│ "); err != nil {
						return err
					}
				}
			}

			cols[ci] = e
		}

		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}

		for ci, col := range w.cols {
			if !col.SeparateLine || len(cols[ci]) == 0 {
				continue
			}

			if _, err := fmt.Fprintf(out, "  %s: %s\n", col.Name, cols[ci]); err != nil {
				return err
			}
		}

		if lineNumber == 0 && cfg.borders {
			// print bottom of header
			if _, err := fmt.Fprint(out, "├"); err != nil {
				return err
			}
			for ci, col := range w.cols {
				if col.Lines == 0 {
					continue
				}

				if _, err := fmt.Fprint(out, strings.Repeat("─", colLengths[ci]+2)); err != nil {
					return err
				}
				if ci != len(w.cols)-1 {
					if _, err := fmt.Fprint(out, "┼"); err != nil {
						return err
					}
				}
			}
			if _, err := fmt.Fprintln(out, "┤"); err != nil {
				return err
			}
		}
	}

	if cfg.borders {
		// bottom line
		if _, err := fmt.Fprint(out, "└"); err != nil {
			return err
		}
		for ci, col := range w.cols {
			if col.Lines == 0 {
				continue
			}
			if _, err := fmt.Fprint(out, strings.Repeat("─", colLengths[ci]+2)); err != nil {
				return err
			}
			if ci != len(w.cols)-1 {
				if _, err := fmt.Fprint(out, "┴"); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintln(out, "┘"); err != nil {
			return err
		}
	}

	return nil
}

func cliStringLength(s string) (n int) {
	return utf8.RuneCountInString(stripansi.Strip(s))
}
