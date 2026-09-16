package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
)

// printJSON writes v as indented JSON to out.
func printJSON(out io.Writer, v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", b)
	return err
}

// newTable returns a writer with the analyst-suite defaults.
func newTable(out io.Writer, header []string) *tablewriter.Table {
	t := tablewriter.NewWriter(out)
	t.SetHeader(header)
	t.SetAutoWrapText(false)
	t.SetHeaderLine(false)
	t.SetAutoFormatHeaders(false)
	return t
}

// emit writes v as JSON when isJSON is true, otherwise as a table.
func emit(out io.Writer, isJSON bool, v interface{}, header []string, rows [][]string) error {
	if isJSON {
		return printJSON(out, v)
	}
	t := newTable(out, header)
	t.AppendBulk(rows)
	t.Render()
	return nil
}

// money formats an amount in units of one hundred million yuan.
func money(v float64) string {
	return fmt.Sprintf("%.2f亿", v/1e8)
}

// earningsAmount formats a forecast band value according to what is being
// forecast. Per-share figures such as 每股收益 are already in yuan, so
// dividing them by 1e8 would flatten them to zero.
func earningsAmount(item string, v float64) string {
	if strings.Contains(item, "每股收益") {
		return fmt.Sprintf("%.2f元", v)
	}
	return money(v)
}

// earningsBand renders a forecast bound, using "-" for the empty records
// Eastmoney returns for old announcements that never published a band.
func earningsBand(item string, v float64) string {
	if v == 0 {
		return "-"
	}
	return earningsAmount(item, v)
}

// num formats a float with the given decimals.
func num(v float64, dec int) string {
	return fmt.Sprintf("%.*f", dec, v)
}

// pct formats a fraction as a percentage.
func pct(v float64, dec int) string {
	return fmt.Sprintf("%.*f%%", dec, v*100)
}

// signPct formats a fraction as a signed percentage.
func signPct(v float64, dec int) string {
	return fmt.Sprintf("%+.*f%%", dec, v*100)
}

// pctOf formats a value that is already expressed in percentage points.
// Eastmoney's change_pct and turnover_rate arrive this way, as do the
// stock daily pct_chg column.
func pctOf(v float64, dec int) string {
	return fmt.Sprintf("%.*f%%", dec, v)
}

// signPctOf formats a value that is already in percentage points.
func signPctOf(v float64, dec int) string {
	return fmt.Sprintf("%+.*f%%", dec, v)
}

// changeOf formats an Eastmoney growth band value. The datacenter returns 0
// when a forecast carries no growth figure, so an unconditional formatter would
// print a misleading +0.0%.
func changeOf(v float64, dec int) string {
	if v == 0 {
		return "-"
	}
	return signPctOf(v, dec)
}

// ratio returns the quotient of a over b, 0 when b is zero.
func ratio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// writeSVG saves an SVG document to path and reports where.
func writeSVG(path, body string) error {
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return fmt.Errorf("write svg: %w", err)
	}
	return nil
}
