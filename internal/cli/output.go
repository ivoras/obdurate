package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type OutputMode int

const (
	OutputTable OutputMode = iota
	OutputJSON
	OutputCSV
)

type Printer struct {
	Out  io.Writer
	Err  io.Writer
	Mode OutputMode
}

func NewPrinter() *Printer {
	return &Printer{Out: os.Stdout, Err: os.Stderr, Mode: OutputTable}
}

func (p *Printer) SetFlags(jsonOut, csvOut bool) error {
	if jsonOut && csvOut {
		return fmt.Errorf("use only one of --json or --csv")
	}
	if jsonOut {
		p.Mode = OutputJSON
	} else if csvOut {
		p.Mode = OutputCSV
	} else {
		p.Mode = OutputTable
	}
	return nil
}

func (p *Printer) PrintJSON(v any) error {
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p *Printer) PrintOK(msg string) {
	if p.Mode == OutputTable {
		fmt.Fprintln(p.Out, msg)
	}
}

func (p *Printer) PrintTable(headers []string, rows [][]string) error {
	switch p.Mode {
	case OutputJSON:
		objs := make([]map[string]string, 0, len(rows))
		for _, r := range rows {
			m := map[string]string{}
			for i, h := range headers {
				if i < len(r) {
					m[h] = r[i]
				}
			}
			objs = append(objs, m)
		}
		return p.PrintJSON(objs)
	case OutputCSV:
		w := csv.NewWriter(p.Out)
		if err := w.Write(headers); err != nil {
			return err
		}
		if err := w.WriteAll(rows); err != nil {
			return err
		}
		w.Flush()
		return w.Error()
	default:
		tw := tabwriter.NewWriter(p.Out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, strings.Join(headers, "\t"))
		for _, r := range rows {
			fmt.Fprintln(tw, strings.Join(r, "\t"))
		}
		return tw.Flush()
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func joinCSV(ss []string) string {
	return strings.Join(ss, ",")
}

func parseIDArg(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", s)
	}
	return id, nil
}

func splitCSVFlag(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	// map known store errors loosely via message prefix
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"):
		return 2
	case strings.Contains(msg, "already exists"), strings.Contains(msg, "conflict"), strings.Contains(msg, "invalid"):
		return 3
	default:
		return 1
	}
}
