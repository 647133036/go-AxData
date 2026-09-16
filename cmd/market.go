package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/electkismet/axdata-go/core/chart"
	"github.com/electkismet/axdata-go/core/market"
	"github.com/spf13/cobra"
)

func newMarketCmd(r *RootCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "market",
		Short: "Real-time quotes, kline and technical charts",
	}
	cmd.AddCommand(newMarketQuoteCmd(r))
	cmd.AddCommand(newMarketChartCmd(r))
	cmd.AddCommand(newMarketWatchCmd(r))
	return cmd
}

func newMarketSvc(r *RootCmd) *market.Service {
	return &market.Service{Getter: r.getter}
}

func newMarketQuoteCmd(r *RootCmd) *cobra.Command {
	var isJSON bool
	c := &cobra.Command{
		Use:   "quote CODE [CODE...]",
		Short: "Print a real-time quote snapshot",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := r.runQuote(ctx, args, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runQuote(ctx context.Context, codes []string, isJSON bool) error {
	quotes, err := newMarketSvc(r).Quote(ctx, codes...)
	if err != nil {
		return err
	}
	if len(quotes) == 0 {
		return fmt.Errorf("no quote data returned")
	}
	if isJSON {
		return printJSON(os.Stdout, quotes)
	}
	rows := [][]string{}
	for _, q := range quotes {
		rows = append(rows, []string{
			q.Code, q.Name,
			num(q.LastPrice, 2), signPctOf(q.ChangePct, 2),
			fmt.Sprintf("%.2f万", q.Volume/1e4), money(q.Amount),
			pctOf(q.TurnoverRate, 2), num(q.PE, 2), num(q.PB, 2),
			q.Source,
		})
	}
	return emit(os.Stdout, false, nil,
		[]string{"代码", "名称", "现价", "涨跌%", "成交量(万手)", "成交额", "换手%", "PE", "PB", "来源"}, rows)
}

func newMarketChartCmd(r *RootCmd) *cobra.Command {
	var (
		isJSON            bool
		limit, rows, cols int
		svgPath           string
	)
	c := &cobra.Command{
		Use:   "chart CODE",
		Short: "Render a candlestick chart with MA/RSI/MACD",
		Long:  "Print an ASCII candlestick chart with moving averages, RSI and MACD panes, or export a standalone SVG.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := r.runChart(ctx, args[0], limit, rows, cols, svgPath, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().IntVar(&limit, "bars", 120, "number of trading bars to fetch")
	c.Flags().IntVar(&rows, "height", 30, "ASCII chart height in rows")
	c.Flags().IntVar(&cols, "width", 110, "ASCII chart width in columns")
	c.Flags().StringVar(&svgPath, "svg", "", "also write a standalone SVG file to this path")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output the underlying chart data as JSON")
	return c
}

func (r *RootCmd) runChart(ctx context.Context, code string, limit, rows, cols int,
	svgPath string, isJSON bool) error {

	svc := newMarketSvc(r)
	bars, err := svc.Kline(ctx, market.KlineOption{Code: code, Limit: limit})
	if err != nil {
		return err
	}
	if len(bars) == 0 {
		return fmt.Errorf("no kline data for %s", code)
	}

	chartData, err := market.BuildChart(code, "", bars)
	if err != nil {
		return err
	}

	if isJSON {
		out := map[string]interface{}{
			"code":   code,
			"bars":   chartData.Len(),
			"first":  chartData.Dates[0],
			"last":   market.LastBarDate(bars),
			"detail": chartData.DetailTable(10),
		}
		return printJSON(os.Stdout, out)
	}

	if svgPath != "" {
		if err := writeSVG(svgPath, chart.RenderSVG(chartData, 1200, 640)); err != nil {
			return err
		}
		fmt.Printf("SVG written to %s\n\n", svgPath)
	}
	fmt.Print(chart.RenderASCII(chartData, rows, cols))
	fmt.Println()
	fmt.Printf("最新交易日 %s  收盘 %.2f  MA5 %.2f  MA20 %.2f  RSI %.1f\n",
		market.LastBarDate(bars),
		chartData.Close[chartData.Len()-1],
		chartData.MA5[chartData.Len()-1],
		chartData.MA20[chartData.Len()-1],
		chartData.RSI[chartData.Len()-1])
	return nil
}

func newMarketWatchCmd(r *RootCmd) *cobra.Command {
	var (
		isJSON bool
		limit  int
		codes  []string
	)
	c := &cobra.Command{
		Use:   "watch",
		Short: "Watch a list of securities with quotes and a summary table",
		Long:  "Fetch a real-time snapshot for a watchlist and print an aligned summary. Reads --codes repeatedly or a comma-separated value.",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			merged := []string{}
			for _, raw := range codes {
				merged = append(merged, strings.Split(raw, ",")...)
			}
			merged = compact(merged)
			if err := r.runWatch(ctx, merged, limit, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().StringSliceVar(&codes, "codes", nil, "watchlist codes, repeatable and comma-separated")
	c.Flags().IntVar(&limit, "bars", 60, "bars fetched for each security")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runWatch(ctx context.Context, codes []string, limit int, isJSON bool) error {
	if len(codes) == 0 {
		return fmt.Errorf("no codes given; use --codes")
	}
	type item struct {
		Code      string
		Name      string
		Price     float64
		ChangePct float64
		High      float64
		Low       float64
		MA20      float64
		Bias      float64
		Range     float64
		PE        float64
		PB        float64
		Src       string
	}
	items := []item{}
	svc := newMarketSvc(r)
	for _, code := range codes {
		qs, err := svc.Quote(ctx, code)
		if err != nil {
			return fmt.Errorf("quote %s: %w", code, err)
		}
		if len(qs) == 0 {
			continue
		}
		q := qs[0]
		it := item{Code: code, Name: q.Name, Price: q.LastPrice, ChangePct: q.ChangePct,
			High: q.High, Low: q.Low, PE: q.PE, PB: q.PB, Src: q.Source}
		if it.Price > 0 {
			it.Range = (it.High - it.Low) / it.Price
		}
		bars, err := svc.Kline(ctx, market.KlineOption{Code: code, Limit: limit})
		if err == nil && len(bars) >= 20 {
			cd, err := market.BuildChart(code, "", bars)
			if err == nil {
				it.MA20 = cd.MA20[cd.Len()-1]
				if it.MA20 > 0 {
					it.Bias = (it.Price - it.MA20) / it.MA20
				}
			}
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return fmt.Errorf("no quote data returned")
	}
	if isJSON {
		return printJSON(os.Stdout, items)
	}
	rows := [][]string{}
	for _, it := range items {
		rows = append(rows, []string{
			it.Code, it.Name,
			num(it.Price, 2), signPctOf(it.ChangePct, 2),
			pct(it.Bias, 2), num(it.MA20, 2),
			fmt.Sprintf("%.2f~%.2f", it.Low, it.High),
			pct(it.Range, 2), num(it.PE, 2), num(it.PB, 2),
		})
	}
	return emit(os.Stdout, false, nil,
		[]string{"代码", "名称", "现价", "涨跌%", "乖离20", "MA20", "日内区间", "振幅", "PE", "PB"}, rows)
}

func compact(in []string) []string {
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
