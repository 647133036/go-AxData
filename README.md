# AxData-go

Go 语言版 AxData —— A股量化数据收集与分析平台，兼容 Python 版 [AxData](https://github.com/axdata/axdata) 的数据模型和 API 规范。

## 特性

- **11 个数据源适配器**：TDX 7709（通达信行情）、TDX ExHq 7727（通达信扩展行情）、东财、同花顺、新浪财经、腾讯财经、财联社、巨潮资讯、看盘汇、文财、mock
- **105 个标准接口**：覆盖日线、分钟线、实时行情、板块、题材、龙虎榜、财报、扩展行情等全量数据
- **69 张表结构定义**：标准化的 Schema Registry，支持 FieldMapping
- **标准化 ETL 管道**：`Adapter → ProviderRegistry → FieldMapping → Schema → DuckDB/Parquet`
- **5 个分析模块**：行情图表、基本面、业绩预告、DCF 估值、组合分析，全部支持 JSON 输出
- **本地优先缓存**：Parquet 优先、源回源、全源失败时降级到陈旧数据，快照表跨标的隔离
- **纯 Go TCP 协议**：7709 行情 + 7727 扩展行情双协议，零外部 TDX SDK 依赖
- **Go workspace 多模块**：12 个 Go module，每个数据源独立模块，核心模块统一维护
- **完整测试覆盖**：17 个 core 包 + 10 个数据源模块，335 个测试函数，全部通过（含 `-race`）

## 架构

```
┌──────────────────┐     ┌───────────────────┐     ┌────────────────┐
│ Source Adapters   │     │ ProviderRegistry   │     │ Schema Table   │
│ (11 adapters)     │───▶│  (105 interfaces)  │───▶│  (69 tables)   │
│                   │     │  Field Mapping     │     │  Parquet/DB    │
│ TDX 7709 ◀──┐    │     └──────────┬────────┘     └────────┬───────┘
│ TDX ExHq 7727   │                │                       │
│ Tencent/Sina/.. │                ▼                       ▼
│                  │       ┌──────────────────┐   ┌────────────────┐
└──────────────────┘       │  Collector       │   │  Querier       │
                            │  Task Management  │   │  DuckDB SQL     │
                            └──────────────────┘   └────────────────┘
```

## 安装

```bash
git clone https://github.com/647133036/go-AxData.git
cd go-AxData
go build -o axdata .
```

Go 1.24+

## 快速开始

```bash
# 查看数据源列表
./axdata sources list

# 查看可用表和接口
./axdata data list

# 添加采集任务
./axdata collector add \
  --name "daily" \
  --source tencent \
  --interface stock_zh_a_hist_tx \
  --table daily \
  --params '{"codes":"000001.SZ,000002.SZ","period":"daily"}'

# 启动 API 服务
./axdata api --port 8080
```

## 数据源

| 数据源 | 类型 | 协议 | 接口数 | 说明 |
|--------|------|------|--------|------|
| **TDX**（通达信行情） | 行情 | TCP 7709 二进制 | 8 | 日线/分钟线、连板天梯、ST/停牌列表、题材强度排行；多主机故障自动转移 |
| **TDX ExHq**（通达信扩展行情） | 行情 | TCP 7727 二进制 | 6 | 扩展市场列表、证券列表（15万+）、历史K线、实时行情快照、分类报价 |
| **腾讯财经** | HTTP | HTTP API | 5 | 实时行情快照、个股/指数日K线、逐笔成交 |
| **新浪财经** | HTTP | HTTP API | 4 | 实时行情、日K/周K/月K/年K、板块排行 |
| **东方财富** | HTTP | HTTP API | 15 | 实时行情、龙虎榜、融资融券、研报、涨跌停池、异动、交易日 |
| **财联社** | HTTP | HTTP API | 17 | 市场情绪/风向、热门板块/概念/个股、涨停池、行业排行、市场主线 |
| **开盘红** | HTTP | HTTP API | 10 | 板块排行、概念详情、连板天梯、市场复盘、涨停/跌停历史 |
| **同花顺** | HTTP | HTTP API | 1 | 人气榜 |
| **巨潮资讯** | HTTP | HTTP API | 32 | 公司档案、公告、分红、股东、股权质押、债券、基金持仓 |
| **文财（i问财）** | HTTP | HTTP API | 1 | 自然语言选股策略查询 |
| **mock** | 本地 | 模拟 | 1 | 测试用模拟数据 |

## 核心概念

### 数据流

```
Adapter.Request() → ProviderRegistry → FieldMapping → Schema → Storage (Parquet)
                                                          ↘  Querier (DuckDB SQL)
```

每个数据源实现统一的 `Adapter` 接口：
```go
type Adapter interface {
    Name() string
    Description() string
    Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error)
}
```

### TDX 双协议

TDX 适配器同时支持 7709（标准行情）和 7727（扩展行情 ExHq）两套二进制协议：

- **7709**：A 股主板/创业板/科创板行情，日线/分钟线，财务数据，价格限幅
- **7727 ExHq**：扩展行情，覆盖 52 个市场（含期货、期权、港股、指数），15 万+ 证券，历史 K 线和实时快照

两个适配器共享 `source-tdx` 模块，独立注册，各自维护连接和故障转移。ExHq 适配器实现了完整的 92 字节 setup 握手帧，与 pytdx 字节对齐。

### 采集任务

```bash
# 添加任务
./axdata collector add \
  --name "tencent-000001" \
  --source tencent \
  --interface stock_zh_a_hist_tx \
  --table daily \
  --params '{"codes":"000001.SZ","period":"daily"}'

# 查看任务列表
./axdata collector list

# 执行采集
./axdata collector run

# 查看任务状态
./axdata collector status
```

### 查询

```bash
# DuckDB SQL 查询
./axdata query "SELECT * FROM daily WHERE trade_date > '2026-01-01' LIMIT 10"

# API 查询
curl "http://localhost:8080/api/query?sql=SELECT * FROM daily LIMIT 10"
```

## 分析命令

在原始数据之上提供 5 个分析模块，全部支持 `--format-json` 输出机器可读结果（写入 stdout；状态横幅写入 stderr，管道不受污染）。

| 命令 | 说明 |
|------|------|
| `market quote CODE` | 实时行情快照：最新价、涨跌幅、成交量额、换手率、PE/PB |
| `market watch --codes A,B,C` | 多标的横向对比，按涨跌幅降序排列，含乖离率、日内区间与振幅 |
| `market chart CODE` | 本地 K 线 + MA5/10/20 + RSI(14) + MACD 的 ASCII 图表，可导出 SVG |
| `fundamental profile CODE` | 财务报表指标、主营构成（按维度分解）、估值快照、ROE 与盈利质量 |
| `earnings report CODE` | 业绩预告与预告后实际业绩对比：变动幅度、是否超预期及原因说明 |
| `valuation dcf CODE` | 两阶段 DCF：内在价值、安全边际、WACC/终值增长率敏感性矩阵 |
| `portfolio analyze` | 组合波动率分解、风险贡献占比、HHI 集中度、相关性矩阵 |

```bash
# 单只股票基本面
./axdata fundamental profile 600519.SH

# K 线图（60 根 K 线，宽 130 列）
./axdata market chart 600519.SH --bars 60 --width 130 --svg chart.svg

# 业绩预告
./axdata earnings report 600519.SH

# DCF 估值（自定义 WACC 与永续增长率）
./axdata valuation dcf 600519.SH --wacc 0.08 --terminal-growth 0.02

# 组合分析（等权，或指定权重）
./axdata portfolio analyze --weights 600519.SH=50,000858.SZ=30,000001.SZ=20

# 机器可读输出
./axdata valuation dcf 600519.SH --format-json
```

### 本地优先缓存

分析命令的数据读取统一走 `core/cache`，顺序为：本地 Parquet → 数据源抓取 → 写回缓存。全部数据源失败时返回本地陈旧数据并标注，避免分析能力因网络问题完全不可用。

快照类表（财务三表、主营构成、业绩预告、估值快照）会跨标的共享同一个文件，读取时按 `ts_code` 过滤，写回时合并其他标的的行，保证多标的缓存互不覆盖。

## 输出格式

| 格式 | 路径 | 说明 |
|------|------|------|
| Parquet | `$AXDATA_ROOT/data/<layer>/<table>.parquet` | 列式存储，适合分析 |
| DuckDB | 内存 | 通过 Querier 提供 SQL 查询 |

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AXDATA_ROOT` | `./axdata_data` | 数据根目录 |
| `AXDATA_TDX_HOSTS` | 内置 7709 服务器列表 | TDX 行情主机，逗号分隔 `host:port` |
| `AXDATA_TDXEX_HOSTS` | 内置 7727 服务器列表 | TDX 扩展行情主机，逗号分隔 `host:port` |

## 测试

```bash
# 全量测试（29 个包）
go test -count=1 -timeout 120s \
  . ./cmd/... ./core/... \
  ./source-cls/... ./source-cninfo/... ./source-eastmoney/... \
  ./source-kph/... ./source-mock/... ./source-sina/... \
  ./source-tdx/... ./source-tencent/... ./source-ths/... \
  ./source-wencai/...

# 含竞态检测
go test -count=1 -race -timeout 180s \
  . ./cmd/... ./core/... \
  ./source-cls/... ./source-cninfo/... ./source-eastmoney/... \
  ./source-kph/... ./source-mock/... ./source-sina/... \
  ./source-tdx/... ./source-tencent/... ./source-ths/... \
  ./source-wencai/...

# 集成测试（需要网络连接 TDX 服务器）
go test -v -count=1 -run TestIntegration .

# Live TDX 测试（7709 + 7727）
cd source-tdx && go test -v -count=1 -run TestLive .
```

当前 17 个 core 包 + 10 个数据源模块，335 个测试函数，全部通过（含 `-race`）。

## 开发

### 添加新数据源

```bash
# 1. 创建模块
mkdir source-mysource

# 2. 实现 Adapter
cat > source-mysource/mysource.go << 'EOF'
package mysource

import "context"

type MySourceAdapter struct{}

func NewMySourceAdapter() *MySourceAdapter { return &MySourceAdapter{} }
func (a *MySourceAdapter) Name() string    { return "mysource" }
func (a *MySourceAdapter) Description() string { return "MySource Adapter" }
func (a *MySourceAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
    // 实现数据获取逻辑
    return nil, nil
}
EOF

# 3. 在 main.go 中注册
source.Register(mysource.NewMySourceAdapter())
```

### 项目结构

```
├── main.go              # 入口，注册所有适配器
├── cmd/                 # CLI 命令
├── core/                # 核心模块
│   ├── api/             # API 服务
│   ├── cache/           # 本地优先缓存（含跨标的快照隔离）
│   ├── chart/           # ASCII / SVG 图表渲染
│   ├── collector/       # 采集任务系统
│   ├── config/          # 配置管理
│   ├── earnings/        # 业绩预告与业绩对比
│   ├── fundamental/     # 财务指标与主营构成
│   ├── indicator/       # 技术指标（MA/RSI/MACD）
│   ├── market/          # 行情、K 线、图表数据
│   ├── portfolio/       # 组合波动率与集中度
│   ├── plugin/          # 插件系统
│   ├── query/           # DuckDB 查询器
│   ├── schema/          # 表结构定义（69 张表）
│   ├── source/          # Provider 注册表（105 个接口）
│   ├── storage/         # Parquet 存储
│   └── valuation/       # DCF 估值模型
├── source-*/            # 10 个数据源模块（11 个适配器）
├── integration_test.go  # 集成测试
└── go.work              # Go workspace（12 个模块）
```

## 变更日志

### v2.0.2

- 修复 TDX 三个解析器的游标推进失效：`closeRaw, pos := varint(...)` 在循环体内声明了块级 `pos`，遮蔽外层游标，导致第 2 条记录起全部按错误偏移解码。影响 `parseKlineRows`（多周期 K 线）、`parseQuotesRows`（多标的行情）、`parseCategoryQuoteRows`（分类报价/连板天梯/题材强度）
- 修复 `parsePriceLimitsRows` 记录长度按 11 字节切分导致 `binary.LittleEndian.Uint32` 越界 panic，改为 15 字节记录（market u8 + code 6B + up u32 + down u32）
- `parseKlineRows` 增加 `index` 参数：指数 K 线的 4 字节上涨/下跌家数仅在指数响应中读取，输出为 `up_count`/`down_count`
- `intval` 支持 `float64`/`float32` 入参
- `source-kph`：`post()` 的 `errcode` 判定改为按字符串比较（JSON 数字是 `float64`，与 `0` 比较恒不相等），新增 `paramAsInt`
- 补齐 HTTP 状态码检查：cls `httpGet`、wencai 取 cookie 与查询、eastmoney 交易日历两处、ths 热榜、sina 指数列表 GET、tencent 四处分页、core/plugin/tencent 两处
- 新增解析器回归测试：多标的行情、分类报价、多周期 K 线（含指数家数）、涨跌停价、varint 往返
- 新增 `TestProbe_QuoteCommandsNotServed` 记录实测结论：所有可达的 7709 服务器都不再返回多标的实时行情，pytdx 参考实现同样返回 0 行
- 修复 `TestSchedulerStartupRunsOnce` 竞态：固定 sleep 可能落在 run 记录已发布、`MarkRuns` 尚未提交 `LastRun` 的窗口内，改为 `eventually` 轮询两个条件
- 修复 `TestRequestIntervalZeroNoThrottle` 在 `-race` 下偶发失败：新增 `throttle` 表为空的结构性断言，墙钟间隙阈值放宽到 500ms 并标注抖动容差

### v2.0.1

- 新增 TDX ExHq 7727 扩展行情适配器：6 个接口（市场列表、证券列表、历史K线、实时快照、分类报价、证券计数），覆盖 52 个市场、15 万+ 证券
- 全项目代码审查与安全审计：修复 30 个 bug（1 个 critical、12 个 high、10 个 medium、7 个 low）
  - 修复 varint 多字节解析 `pos` 越界导致 K 线/行情数据错乱
  - 修复腾讯 `code != 0` 与 JSON `float64(0)` 类型比较失效导致所有成功响应被误判为错误
  - 修复新浪 `parseRealTime` 越界 panic（`fields[30]`/`fields[31]`）
  - 修复财联社 12 处未检查类型断言导致非对象 JSON 响应 panic
  - 修复 `gbk2str` 未解码 GBK 导致中文股票名乱码
  - 修复 DCF `CashFlowSeries` 未按报告日期排序导致估值错误
  - 修复 `ValidateSQL` 拒绝列表可通过 tab/`COPY(` 绕过
  - 修复 `collector.RunTask` 释放 RLock 后访问 `*Task` 数据竞争
  - 修复 7 个 HTTP 适配器未检查 `resp.StatusCode` 导致 429/5xx 被静默吞为成功
  - 修复腾讯 `cleanSymbol` 将 BSE 920xxx 路由到 SSE
  - 修复 `parseFinanceInfoRows`/`parseTodayTradesRows` 长度检查不足导致截断响应 panic
  - 修复 `compactFloat` 使用 `volumeRaw` 而非 `amountRaw` 导致 K 线成交额错误
  - 修复财联社新闻请求使用 UTC 而非北京时间导致日期偏移 8 小时
  - 修复 `intval`/`splitCSV`/`enrichPrices` 等辅助函数错误处理
  - 修复 `earnings` 业绩对比 verdict 枚举错误
  - 新增 `validIdentifier` 防 SQL 注入，`ValidateSQL` 空白归一化
- 接口数 99 → 105，表数 63 → 69，测试函数 247 → 335

### v2.0.0

- 初始发布：10 个数据源适配器、99 个接口、63 张表、5 个分析模块

## 许可证

GNU Affero GPL v3 (AGPL-3.0-only)，完整文本见仓库根目录 `LICENSE`。

版权 2026 AxData-go 贡献者。

采用 AGPL 是为了引入 AGPL 许可的技术指标库 `github.com/cinar/indicator/v2`。AGPL 的附加义务：修改本项目的源码后，必须以 AGPL 兼容许可发布；通过网络向用户提供服务时，还需向该用户同时提供对应版本的完整对应源码（第 13 条）。
