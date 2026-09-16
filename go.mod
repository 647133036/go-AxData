module github.com/electkismet/axdata-go

go 1.24.0

require (
	github.com/electkismet/axdata-go/core v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-cls v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-cninfo v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-eastmoney v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-kph v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-mock v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-sina v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-tdx v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-tencent v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-ths v0.0.0-00010101000000-000000000000
	github.com/electkismet/axdata-source-wencai v0.0.0-00010101000000-000000000000
	github.com/olekukonko/tablewriter v0.0.5
	github.com/spf13/cobra v1.8.1
	go.uber.org/zap v1.27.0
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/apache/arrow-go/v18 v18.1.0 // indirect
	github.com/duckdb/duckdb-go-bindings v0.1.9 // indirect
	github.com/duckdb/duckdb-go-bindings/darwin-amd64 v0.1.4 // indirect
	github.com/duckdb/duckdb-go-bindings/darwin-arm64 v0.1.4 // indirect
	github.com/duckdb/duckdb-go-bindings/linux-amd64 v0.1.4 // indirect
	github.com/duckdb/duckdb-go-bindings/linux-arm64 v0.1.4 // indirect
	github.com/duckdb/duckdb-go-bindings/windows-amd64 v0.1.4 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/google/flatbuffers v25.1.24+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/marcboeker/go-duckdb/arrowmapping v0.0.2 // indirect
	github.com/marcboeker/go-duckdb/mapping v0.0.2 // indirect
	github.com/marcboeker/go-duckdb/v2 v2.0.0 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/parquet-go/parquet-go v0.23.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/segmentio/encoding v0.4.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/exp v0.0.0-20250128182459-e0ece0dbea4c // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
)

replace (
	github.com/electkismet/axdata-go/core => ./core
	github.com/electkismet/axdata-source-cls => ./source-cls
	github.com/electkismet/axdata-source-cninfo => ./source-cninfo
	github.com/electkismet/axdata-source-eastmoney => ./source-eastmoney
	github.com/electkismet/axdata-source-kph => ./source-kph
	github.com/electkismet/axdata-source-mock => ./source-mock
	github.com/electkismet/axdata-source-sina => ./source-sina
	github.com/electkismet/axdata-source-tdx => ./source-tdx
	github.com/electkismet/axdata-source-tencent => ./source-tencent
	github.com/electkismet/axdata-source-ths => ./source-ths
	github.com/electkismet/axdata-source-wencai => ./source-wencai
)
