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

	"github.com/toon-format/toon-go"
)

type OutputMode int

const (
	OutputTable OutputMode = iota
	OutputJSON
	OutputCSV
	OutputTOON
)

type Printer struct {
	Out  io.Writer
	Err  io.Writer
	Mode OutputMode
}

func NewPrinter() *Printer {
	return &Printer{Out: os.Stdout, Err: os.Stderr, Mode: OutputTable}
}

func (p *Printer) SetFlags(jsonOut, csvOut, toonOut bool) error {
	n := 0
	if jsonOut {
		n++
	}
	if csvOut {
		n++
	}
	if toonOut {
		n++
	}
	if n > 1 {
		return fmt.Errorf("use only one of --json, --csv, or --toon")
	}
	switch {
	case jsonOut:
		p.Mode = OutputJSON
	case csvOut:
		p.Mode = OutputCSV
	case toonOut:
		p.Mode = OutputTOON
	default:
		p.Mode = OutputTable
	}
	return nil
}

func (p *Printer) PreferStructured() bool {
	return p.Mode == OutputJSON || p.Mode == OutputTOON
}

func (p *Printer) PrintStructured(v any) error {
	switch p.Mode {
	case OutputTOON:
		return p.PrintTOON(v)
	default:
		return p.PrintJSON(v)
	}
}

func (p *Printer) PrintJSON(v any) error {
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p *Printer) PrintTOON(v any) error {
	// Round-trip via JSON so field names match --json (json struct tags).
	jb, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var generic any
	if err := json.Unmarshal(jb, &generic); err != nil {
		return err
	}
	b, err := toon.Marshal(generic, toon.WithIndent(2))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(p.Out, string(b))
	return err
}

func (p *Printer) PrintOK(msg string) {
	if p.Mode == OutputTable {
		fmt.Fprintln(p.Out, msg)
	}
}

func (p *Printer) PrintTable(headers []string, rows [][]string) error {
	switch p.Mode {
	case OutputJSON, OutputTOON:
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
		return p.PrintStructured(objs)
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
