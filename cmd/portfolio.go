package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/electkismet/axdata-go/core/market"
	"github.com/electkismet/axdata-go/core/portfolio"
	"github.com/spf13/cobra"
)

const portfolioMinReturns = 20

func newPortfolioCmd(r *RootCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "portfolio",
		Short: "Portfolio diversification and concentration analysis",
	}
	cmd.AddCommand(newPortfolioAnalyzeCmd(r))
	return cmd
}

func newPortfolioAnalyzeCmd(r *RootCmd) *cobra.Command {
	var (
		isJSON     bool
		bars       int
		holdings   []string
		weightsRaw string
	)
	c := &cobra.Command{
		Use:   "analyze",
		Short: "Analyse diversification for a set of holdings",
		Long: `Analyse portfolio diversification.

Holdings are given as --weight CODE=PERCENT pairs, e.g.
  --weight 000001.SZ=50 --weight 600519.SH=50
or as one comma-separated --weights value of the same pairs.

Return series are built from daily closes. Weights are normalised to sum to
one, so absolute sizes do not matter.`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if len(holdings) == 0 && weightsRaw != "" {
				holdings = strings.Split(weightsRaw, ",")
			}
			if err := r.runPortfolio(ctx, holdings, bars, isJSON); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		},
	}
	c.Flags().StringSliceVar(&holdings, "weight", nil, "holding as CODE=PERCENT, repeatable")
	c.Flags().StringVar(&weightsRaw, "weights", "", "comma-separated CODE=PERCENT pairs")
	c.Flags().IntVar(&bars, "bars", 250, "trading days of history per holding")
	c.Flags().BoolVar(&isJSON, "format-json", false, "output JSON instead of a table")
	return c
}

func (r *RootCmd) runPortfolio(ctx context.Context, specs []string, bars int, isJSON bool) error {
	holdings := []portfolio.Holding{}
	seen := map[string]bool{}
	for _, raw := range specs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid holding %q, want CODE=PERCENT", s)
		}
		code := strings.TrimSpace(parts[0])
		w, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return fmt.Errorf("invalid weight for %s: %w", code, err)
		}
		if w < 0 {
			return fmt.Errorf("weight for %s must not be negative", code)
		}
		if seen[code] {
			return fmt.Errorf("duplicate holding %s", code)
		}
		seen[code] = true
		holdings = append(holdings, portfolio.Holding{Code: code, Weight: w})
	}
	if len(holdings) < 2 {
		return fmt.Errorf("at least two holdings required")
	}

	total := 0.0
	for _, h := range holdings {
		total += h.Weight
	}
	if total <= 0 {
		return fmt.Errorf("holdings must have positive weights")
	}
	for i := range holdings {
		holdings[i].Weight /= total
	}

	svc := newMarketSvc(r)
	series := map[string][]float64{}
	for _, h := range holdings {
		kline, err := svc.Kline(ctx, market.KlineOption{Code: h.Code, Limit: bars})
		if err != nil {
			return fmt.Errorf("history for %s: %w", h.Code, err)
		}
		closes := make([]float64, 0, len(kline))
		for _, b := range kline {
			closes = append(closes, b.Close)
		}
		returns := closesToReturns(closes)
		if len(returns) < portfolioMinReturns {
			return fmt.Errorf("not enough history for %s: %d daily returns", h.Code, len(returns))
		}
		series[h.Code] = returns
	}

	res, err := portfolio.Analyze(series, holdings)
	if err != nil {
		return err
	}
	if isJSON {
		return printJSON(os.Stdout, res)
	}

	fmt.Println("组合概览:")
	fmt.Printf("  持仓数量     %d\n", len(res.Codes))
	fmt.Printf("  组合波动率   %s\n", pct(res.PortfolioVolatility, 2))
	fmt.Printf("  集中度 HHI   %.0f\n", res.HHI)
	fmt.Printf("  分散化比率   %.2f\n", res.DiversificationRatio)

	fmt.Println("\n各持仓波动与风险贡献:")
	rows := [][]string{}
	for _, rc := range res.RiskContributions {
		rows = append(rows, []string{
			rc.Code, pct(rc.Weight, 2), pct(rc.Volatility, 2), pct(rc.Percentage, 2),
		})
	}
	emit(os.Stdout, false, nil, []string{"代码", "权重", "波动率", "风险贡献占比"}, rows)

	if len(res.HighCorrelationPairs) > 0 {
		fmt.Printf("\n高相关配对 (相关性 > %.2f):\n", 0.8)
		pairs := append([]portfolio.AssetPair(nil), res.HighCorrelationPairs...)
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].Correlation > pairs[j].Correlation })
		for _, p := range pairs {
			fmt.Printf("  %s ~ %s  相关性 %s\n", p.A, p.B, pct(p.Correlation, 2))
		}
	}

	if over, w, ok := portfolio.MaxSingleWeight(weightMap(holdings)); ok {
		fmt.Printf("\n集中度提示: %s 权重 %s, 超过 %.0f%% 建议上限\n",
			over, pct(w, 1), portfolio.MaxSingleWeightLimit()*100)
	}

	if len(res.Recommendations) > 0 {
		fmt.Println("\n建议:")
		for i, rec := range res.Recommendations {
			fmt.Printf("  %d. %s\n", i+1, rec)
		}
	}
	return nil
}

// closesToReturns converts a close series into simple daily returns.
func closesToReturns(closes []float64) []float64 {
	if len(closes) < 2 {
		return nil
	}
	out := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		if closes[i-1] <= 0 {
			continue
		}
		out = append(out, closes[i]/closes[i-1]-1)
	}
	return out
}

// weightMap converts holdings to a plain weight map.
func weightMap(holdings []portfolio.Holding) map[string]float64 {
	out := map[string]float64{}
	for _, h := range holdings {
		out[h.Code] = h.Weight
	}
	return out
}
