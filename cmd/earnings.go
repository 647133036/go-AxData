package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/electkismet/axdata-go/core/earnings"
	"github.com/spf13/cobra"
)

func newEarningsCmd(r *RootCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "earnings",
		Short: "Profit pre-announcements and forecast versus actual",
	}
	cmd.AddCommand(newEarningsForecastsCmd(r))
	cmd.AddCommand(newEarningsCompareCmd(r))
	cmd.AddCommand(newEarningsReportCmd(r))
	return cmd
}

func newEarningsSvc(r *RootCmd) *earnings.Service {
	return &earnings.Service{Getter: r.getter}
}

func newEarningsForecastsCmd(r *RootCmd) *cobra.Command {
	var isJSON bool
	var limit int
	c := &cobra.Command{
		Use:   "forecasts CODE",
		Short: "List profit pre-announcements",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := r.runForecasts(ctx, args[0], limit, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().IntVar(&limit, "limit", 10, "number of forecasts")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runForecasts(ctx context.Context, code string, limit int, isJSON bool) error {
	fs, err := newEarningsSvc(r).Forecasts(ctx, code, limit)
	if err != nil {
		return err
	}
	if len(fs) == 0 {
		return fmt.Errorf("no forecast data for %s", code)
	}
	if isJSON {
		return printJSON(os.Stdout, fs)
	}
	rows := [][]string{}
	for _, f := range fs {
		rows = append(rows, []string{
			f.NoticeDate, earnings.PeriodLabel(f.ReportDate), f.Item,
			f.TypeLabel, earningsAmount(f.Item, f.AmountLower),
			earningsAmount(f.Item, f.AmountUpper),
			changeOf(f.ChangeLower, 1), changeOf(f.ChangeUpper, 1),
		})
	}
	return emit(os.Stdout, false, nil,
		[]string{"公告日", "报告期", "预告口径", "方向", "下限", "上限", "下限增幅", "上限增幅"}, rows)
}

func newEarningsCompareCmd(r *RootCmd) *cobra.Command {
	var isJSON bool
	var limit int
	c := &cobra.Command{
		Use:   "compare CODE",
		Short: "Compare forecasts with reported results",
		Long:  "Pair each profit pre-announcement with its reported net profit and judge whether it beat or missed the forecast band.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := r.runCompare(ctx, args[0], limit, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().IntVar(&limit, "limit", 10, "number of comparisons")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runCompare(ctx context.Context, code string, limit int, isJSON bool) error {
	cmps, err := newEarningsSvc(r).Compare(ctx, code, limit)
	if err != nil {
		return err
	}
	if len(cmps) == 0 {
		return fmt.Errorf("no comparison data for %s", code)
	}
	if isJSON {
		return printJSON(os.Stdout, cmps)
	}
	rows := [][]string{}
	for _, c := range cmps {
		actual := "-"
		if c.Actual != 0 {
			actual = earningsAmount(c.ForecastItem, c.Actual)
		}
		mid := "-"
		if c.Midpoint != 0 {
			mid = earningsAmount(c.ForecastItem, c.Midpoint)
		}
		beat := "-"
		verdict := c.Verdict.String()
		if c.Verdict == earnings.VerdictNoForecast {
			verdict = c.Reason
		} else {
			beat = signPct(c.BeatPct, 1)
		}
		rows = append(rows, []string{
			c.NoticeDate, earnings.PeriodLabel(c.ReportDate), c.ForecastItem,
			mid, actual, beat, verdict,
		})
	}
	return emit(os.Stdout, false, nil,
		[]string{"公告日", "报告期", "预告口径", "预告中值", "实际值", "偏差", "结论"}, rows)
}

func newEarningsReportCmd(r *RootCmd) *cobra.Command {
	var isJSON bool
	var limit int
	c := &cobra.Command{
		Use:   "report CODE",
		Short: "Print the full earnings view",
		Long:  "Assemble forecasts and comparisons for one security, leading with the most recent comparison.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := r.runEarningsReport(ctx, args[0], limit, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().IntVar(&limit, "limit", 10, "number of periods")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runEarningsReport(ctx context.Context, code string, limit int, isJSON bool) error {
	rep, err := newEarningsSvc(r).Report(ctx, code, limit)
	if err != nil {
		return err
	}
	if isJSON {
		return printJSON(os.Stdout, rep)
	}

	fmt.Printf("代码: %s  名称: %s\n\n", rep.Code, rep.Name)

	if rep.Latest != nil {
		l := rep.Latest
		fmt.Println("最近一期对比:")
		fmt.Printf("  报告期     %s (公告日 %s)\n", earnings.PeriodLabel(l.ReportDate), l.NoticeDate)
		fmt.Printf("  预告口径   %s\n", l.ForecastItem)
		if l.Midpoint != 0 {
			fmt.Printf("  预告中值   %s\n", earningsAmount(l.ForecastItem, l.Midpoint))
		}
		if l.Verdict == earnings.VerdictNoForecast {
			fmt.Printf("  说明       %s\n", l.Reason)
		} else if l.Actual != 0 {
			fmt.Printf("  实际值     %s  偏差 %s  结论 %s\n",
				earningsAmount(l.ForecastItem, l.Actual), signPct(l.BeatPct, 1), l.Verdict.String())
		}
	}

	if len(rep.Forecasts) > 0 {
		fmt.Printf("\n业绩预告历史 (%d 条):\n", len(rep.Forecasts))
		rows := [][]string{}
		for _, f := range rep.Forecasts {
			rows = append(rows, []string{
				f.NoticeDate, earnings.PeriodLabel(f.ReportDate),
				f.Item, f.TypeLabel,
				earningsBand(f.Item, f.AmountLower),
				earningsBand(f.Item, f.AmountUpper),
			})
		}
		emit(os.Stdout, false, nil,
			[]string{"公告日", "报告期", "预告口径", "方向", "下限", "上限"}, rows)
	}
	return nil
}
