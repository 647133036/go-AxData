package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/electkismet/axdata-go/core/fundamental"
	"github.com/spf13/cobra"
)

func newFundamentalCmd(r *RootCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fundamental",
		Short: "Financial statements, revenue drivers and valuation",
	}
	cmd.AddCommand(newFundamentalStatementsCmd(r))
	cmd.AddCommand(newFundamentalDriversCmd(r))
	cmd.AddCommand(newFundamentalValuationCmd(r))
	cmd.AddCommand(newFundamentalProfileCmd(r))
	return cmd
}

func newFundSvc(r *RootCmd) *fundamental.Service {
	return &fundamental.Service{Getter: r.getter}
}

func newFundamentalStatementsCmd(r *RootCmd) *cobra.Command {
	var isJSON bool
	var limit int
	c := &cobra.Command{
		Use:   "statements CODE",
		Short: "Print merged financial statements for a security",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := r.runStatements(ctx, args[0], limit, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().IntVar(&limit, "limit", 8, "number of reporting periods")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runStatements(ctx context.Context, code string, limit int, isJSON bool) error {
	stmts, err := newFundSvc(r).Statements(ctx, code, limit)
	if err != nil {
		return err
	}
	if len(stmts) == 0 {
		return fmt.Errorf("no statement data for %s", code)
	}
	if isJSON {
		return printJSON(os.Stdout, stmts)
	}
	rows := [][]string{}
	for _, s := range stmts {
		var revYoY, profitYoY float64
		if prev := fundamental.SamePeriodLastYear(stmts, s.ReportDate); prev != nil {
			revYoY = yearOverYear(s.Revenue, prev.Revenue)
			profitYoY = yearOverYear(s.NetProfit, prev.NetProfit)
		}
		rows = append(rows, []string{
			fundamental.PeriodOf(s.ReportDate),
			money(s.Revenue), yoyCell(revYoY),
			money(s.NetProfit), yoyCell(profitYoY),
			pct(s.GrossMargin, 2), pct(ratio(s.NetProfit, s.Revenue), 2),
			pct(s.DebtAssetRatio, 4), money(s.OCF),
		})
	}
	return emit(os.Stdout, false, nil,
		[]string{"报告期", "营收", "营收同比", "净利润", "净利同比", "毛利率", "净利率", "资产负债率", "经营现金流"}, rows)
}

// yearOverYear returns the growth rate of cur over prev.
func yearOverYear(cur, prev float64) float64 {
	if prev == 0 {
		return 0
	}
	return (cur - prev) / prev
}

// yoyCell renders a growth rate, keeping the dash when no base period exists.
func yoyCell(v float64) string {
	if v == 0 {
		return "-"
	}
	return signPct(v, 2)
}

func newFundamentalDriversCmd(r *RootCmd) *cobra.Command {
	var isJSON bool
	var limit int
	c := &cobra.Command{
		Use:   "drivers CODE",
		Short: "Print the main-business revenue composition",
		Long:  "Show the top revenue drivers of a security for its most recent reporting period, with revenue share and gross margin.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := r.runDrivers(ctx, args[0], limit, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().IntVar(&limit, "limit", 10, "number of driver rows")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runDrivers(ctx context.Context, code string, limit int, isJSON bool) error {
	drivers, err := newFundSvc(r).Drivers(ctx, code, limit)
	if err != nil {
		return err
	}
	if len(drivers) == 0 {
		return fmt.Errorf("no driver data for %s", code)
	}
	if isJSON {
		return printJSON(os.Stdout, drivers)
	}
	rows := [][]string{}
	for _, d := range drivers {
		rows = append(rows, []string{
			fmt.Sprintf("%d", d.Rank), fundamental.PeriodOf(d.ReportDate),
			fundamental.DimensionName(d.Type),
			d.Item, money(d.Income), pct(d.IncomeRatio, 2),
			pct(d.GrossProfitRatio, 2),
		})
	}
	return emit(os.Stdout, false, nil,
		[]string{"排名", "报告期", "维度", "主营项目", "收入", "收入占比", "毛利率"}, rows)
}

func newFundamentalValuationCmd(r *RootCmd) *cobra.Command {
	var isJSON bool
	c := &cobra.Command{
		Use:   "valuation CODE",
		Short: "Print a valuation multiple snapshot",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := r.runValuation(ctx, args[0], isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runValuation(ctx context.Context, code string, isJSON bool) error {
	v, err := newFundSvc(r).Valuation(ctx, code)
	if err != nil {
		return err
	}
	if isJSON {
		return printJSON(os.Stdout, v)
	}
	rows := [][]string{{
		code, v.Name, v.Industry, v.TradeDate,
		num(v.ClosePrice, 2), money(v.TotalMarketCap), money(v.FreeMarketCap),
		num(v.PERatio, 2), num(v.PBRatio, 2), num(v.PSRatio, 2), num(v.PEG, 2),
	}}
	return emit(os.Stdout, false, nil,
		[]string{"代码", "名称", "行业", "交易日", "收盘价", "总市值", "流通市值", "PE(TTM)", "PB", "PS", "PEG"}, rows)
}

func newFundamentalProfileCmd(r *RootCmd) *cobra.Command {
	var isJSON bool
	var limit int
	c := &cobra.Command{
		Use:   "profile CODE",
		Short: "Print a full company profile with derived metrics",
		Long:  "Assemble statements, revenue drivers, valuation and derived ratios for one security.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := r.runProfile(ctx, args[0], limit, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().IntVar(&limit, "limit", 6, "number of reporting periods")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runProfile(ctx context.Context, code string, limit int, isJSON bool) error {
	p, err := newFundSvc(r).Profile(ctx, code)
	if err != nil {
		return err
	}
	if isJSON {
		return printJSON(os.Stdout, p)
	}
	m := p.Metrics
	v := p.Valuation
	fmt.Printf("代码: %s  名称: %s  行业: %s\n\n", p.Code, p.Name, p.Industry)

	fmt.Println("最近报告期核心指标:")
	fmt.Printf("  报告期        %s\n", fundamental.PeriodOf(m.ReportDate))
	fmt.Printf("  营收          %s  (同比 %s)\n", money(m.Revenue), signPct(m.RevenueYoY, 2))
	fmt.Printf("  净利润        %s  (同比 %s)\n", money(m.NetProfit), signPct(m.NetProfitYoY, 2))
	fmt.Printf("  毛利率        %s   净利率 %s\n", pct(m.GrossMargin, 2), pct(m.NetMargin, 2))
	fmt.Printf("  ROE           %s   ROA %s\n", pct(m.ROE, 2), pct(m.ROA, 2))
	fmt.Printf("  资产负债率    %s\n", pct(m.DebtRatio, 2))
	fmt.Printf("  经营现金流/净利润 %s\n", num(m.OCFToNetProfit, 2))
	if m.TopDriver != "" {
		fmt.Printf("  主营贡献第一  %s (%s)\n", m.TopDriver, pct(m.TopDriverShare, 2))
	}
	fmt.Printf("  前3项收入集中度 %s\n", pct(m.RevenueConcentration, 2))

	if v != nil {
		fmt.Println("\n估值快照:")
		fmt.Printf("  交易日 %s  收盘价 %.2f  总市值 %s\n", v.TradeDate, v.ClosePrice, money(v.TotalMarketCap))
		fmt.Printf("  PE(TTM) %s   PB %s   PS %s   PEG %s\n",
			num(v.PERatio, 2), num(v.PBRatio, 2), num(v.PSRatio, 2), num(v.PEG, 2))
	}
	return nil
}
