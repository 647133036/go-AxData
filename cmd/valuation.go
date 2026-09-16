package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/electkismet/axdata-go/core/fundamental"
	"github.com/electkismet/axdata-go/core/valuation"
	"github.com/spf13/cobra"
)

func newValuationCmd(r *RootCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "valuation",
		Short: "Discounted cash flow valuation",
	}
	cmd.AddCommand(newValuationDCFCmd(r))
	return cmd
}

func newValuationDCFCmd(r *RootCmd) *cobra.Command {
	var (
		isJSON                                               bool
		code                                                 string
		years                                                int
		growth, wacc, terminalGrowth, netDebt, shares, price float64
		periods                                              int
	)
	c := &cobra.Command{
		Use:   "dcf CODE",
		Short: "Run a two-stage FCFF DCF model",
		Long: `Run a two-stage discounted cash flow model.

Cash flows are read from the local financial statement cache: free cash flow
is preferred and operating cash flow is used as a proxy when FCF is negative.
The current price comes from the real-time quote when --price is not given.
Shares outstanding are read from the valuation snapshot when --shares is
not given.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			code = args[0]
			if err := r.runDCF(ctx, code, years, growth, wacc, terminalGrowth, netDebt,
				shares, price, periods, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().IntVar(&years, "years", 0, "explicit forecast years, default 5")
	c.Flags().Float64Var(&growth, "growth", 0, "explicit period growth rate, default 0.08")
	c.Flags().Float64Var(&wacc, "wacc", 0, "discount rate, default 0.09")
	c.Flags().Float64Var(&terminalGrowth, "terminal-growth", 0, "terminal growth rate, default 0.025")
	c.Flags().Float64Var(&netDebt, "net-debt", 0, "net debt to subtract from enterprise value")
	c.Flags().Float64Var(&shares, "shares", 0, "shares outstanding, default from valuation snapshot")
	c.Flags().Float64Var(&price, "price", 0, "current price, default from real-time quote")
	c.Flags().IntVar(&periods, "periods", 8, "number of reporting periods to read")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runDCF(ctx context.Context, code string, years int, growth, wacc,
	terminalGrowth, netDebt, shares, price float64, periods int, isJSON bool) error {

	svc := newFundSvc(r)
	stmts, err := svc.Statements(ctx, code, periods)
	if err != nil {
		return err
	}
	if len(stmts) == 0 {
		return fmt.Errorf("no statement data for %s", code)
	}

	if shares <= 0 {
		v, err := svc.Valuation(ctx, code)
		if err == nil {
			shares = v.TotalShares
		}
	}
	if price <= 0 {
		quotes, err := newMarketSvc(r).Quote(ctx, code)
		if err == nil && len(quotes) > 0 {
			price = quotes[0].LastPrice
		}
	}

	cfs := fundamental.CashFlowSeries(stmts)
	res, err := valuation.DCF(cfs, valuation.Assumptions{
		ExplicitYears:  years,
		GrowthRate:     growth,
		WACC:           wacc,
		TerminalGrowth: terminalGrowth,
		NetDebt:        netDebt,
		SharesOut:      shares,
	}, price)
	if err != nil {
		if errors.Is(err, valuation.ErrPriceRequired) {
			return fmt.Errorf("current price unavailable; pass --price", err)
		}
		if errors.Is(err, valuation.ErrSharesRequired) {
			return fmt.Errorf("shares outstanding unavailable; pass --shares", err)
		}
		return err
	}

	if isJSON {
		return printJSON(os.Stdout, res)
	}

	a := res.Assumptions
	fmt.Printf("代码: %s\n\n", code)
	fmt.Println("假设:")
	fmt.Printf("  明确预测期   %d 年\n", a.ExplicitYears)
	fmt.Printf("  增长假设     %s\n", pct(a.GrowthRate, 2))
	fmt.Printf("  WACC         %s\n", pct(a.WACC, 2))
	fmt.Printf("  永续增长率   %s\n", pct(a.TerminalGrowth, 2))
	fmt.Printf("  净债务       %s\n", money(a.NetDebt))
	fmt.Printf("  总股本       %s\n", fmt.Sprintf("%.4g", a.SharesOut))
	fmt.Printf("  现金流来源   %s\n", res.FreeCashFlowProxy)

	fmt.Println("\n估值结果:")
	fmt.Printf("  明确期现值       %s\n", money(res.ExplicitPeriodPV))
	fmt.Printf("  永续价值现值     %s\n", money(res.TerminalValuePV))
	fmt.Printf("  企业价值         %s\n", money(res.EnterpriseValue))
	fmt.Printf("  股权价值         %s\n", money(res.EquityValue))
	fmt.Printf("  每股内在价值     %.2f\n", res.IntrinsicValuePerShare)
	fmt.Printf("  当前价格         %.2f\n", res.CurrentPrice)
	fmt.Printf("  安全边际         %s\n", signPct(res.MarginOfSafety, 2))

	if len(res.Sensitivity) > 0 {
		fmt.Println("\n敏感性分析 (每股价值, 行为永续增长率 列为WACC):")
		// The grid is indexed [terminal growth][WACC]: each row fixes a growth
		// rate and each column a discount rate.
		rows := [][]string{}
		for _, row := range res.Sensitivity {
			line := []string{pct(row[0].Growth, 2)}
			for _, cell := range row {
				if cell.Value == 0 {
					line = append(line, "-")
					continue
				}
				line = append(line, fmt.Sprintf("%.2f", cell.Value))
			}
			rows = append(rows, line)
		}
		header := []string{"G\\WACC"}
		for _, cell := range res.Sensitivity[0] {
			header = append(header, pct(cell.WACC, 2))
		}
		emit(os.Stdout, false, nil, header, rows)
	}
	return nil
}
