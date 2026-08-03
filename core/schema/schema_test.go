package schema

import "testing"

func TestTableRegistry(t *testing.T) {
	tests := []struct{ table string }{
		{"daily"},
		{"adj_factor"},
		{"trade_cal"},
		{"stock_basic_exchange"},
		{"stock_st_list"},
		{"stock_suspension"},
		{"stock_limit_ladder"},
		{"stock_hot_rank"},
		{"stock_theme_rank"},
		{"stock_daily_share"},
		{"stock_rank"},
		{"sector_rank"},
		{"sector_concept_detail"},
		{"sector_plate_detail"},
		{"sector_emotion_detail"},
		{"market_sentiment"},
		{"stock_strategy"},
		{"stock_price_limit"},
		// cninfo tables
		{"stock_profile_cninfo"},
		{"stock_dividend_cninfo"},
		{"stock_industry_category_cninfo"},
		{"stock_ipo_summary_cninfo"},
		{"cninfo_announcements"},
		{"cninfo_irm"},
		{"stock_hold_change_cninfo"},
		{"bond_corporate_issue_cninfo"},
		{"bond_cov_issue_cninfo"},
		{"bond_local_government_issue_cninfo"},
		{"bond_treasure_issue_cninfo"},
		// eastmoney tables
		{"market_index_realtime"},
		{"market_index_all"},
		{"stock_limit_pool_eastmoney"},
		// cls tables
		{"market_mainline_cls"},
	}

	for _, tc := range tests {
		t.Run(tc.table, func(t *testing.T) {
			schemaDef := GetSchema(tc.table)
			if schemaDef == nil {
				t.Errorf("Schema %s not found in registry", tc.table)
				return
			}
			if schemaDef.Name != tc.table {
				t.Errorf("Schema name mismatch: got %s, want %s", schemaDef.Name, tc.table)
			}
			if len(schemaDef.Columns) == 0 {
				t.Errorf("Schema %s has no columns", tc.table)
			}
		})
	}
}

func TestTableRegistryNames(t *testing.T) {
	names := TableRegistryNames()
	if len(names) == 0 {
		t.Fatal("TableRegistryNames returned empty")
	}
	// Check that all returned names actually exist
	for _, name := range names {
		if GetSchema(name) == nil {
			t.Errorf("Name %s returned but not found in registry", name)
		}
	}
}

func TestDailyRecord(t *testing.T) {
	dailySchema := GetSchema("daily")
	if dailySchema == nil {
		t.Fatal("daily schema not found")
	}
	// Check that expected columns exist
	expectedCols := []string{"ts_code", "trade_date", "open", "high", "low", "close", "pre_close", "change", "pct_chg", "vol", "amount"}
	for _, col := range expectedCols {
		found := false
		for _, c := range dailySchema.Columns {
			if c.Name == col {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Daily schema missing column: %s", col)
		}
	}
}
