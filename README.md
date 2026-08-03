# AxData-go

Go 语言版 AxData —— A股量化数据收集与分析平台，兼容 Python 版 [AxData](https://github.com/axdata/axdata) 的数据模型和 API 规范。

## 特性

- **10 个数据源适配器**：TDX（通达信）、东财、同花顺、新浪财经、腾讯财经、财联社、巨潮资讯、看盘汇、文财、mock
- **99 个标准接口**：覆盖日线、分钟线、实时行情、板块、题材、龙虎榜、财报等全量数据
- **63 张表结构定义**：标准化的 Schema Registry，支持 FieldMapping
- **标准化 ETL 管道**：`Adapter → ProviderRegistry → FieldMapping → Schema → DuckDB/Parquet`
- **零外部 TDX SDK 依赖**：纯 Go TCP 协议实现通达信 7709 接口
- **Go workspace 多模块**：每个数据源独立模块，核心模块统一维护
- **完整测试覆盖**：19 个包，160+ 测试函数，全部通过

## 架构

```
┌─────────────┐     ┌───────────────────┐     ┌────────────────┐
│ Source      │     │ ProviderRegistry   │     │ Schema Table   │
│ Adapters    │───▶│  (99 interfaces)   │───▶│  (63 tables)   │
│ (10 sources)│     │  Field Mapping     │     │  Parquet/DB    │
└─────────────┘     └──────────┬────────┘     └────────┬───────┘
                               │                       │
                               ▼                       ▼
                      ┌──────────────────┐   ┌────────────────┐
                      │  Collector       │   │  Querier       │
                      │  Task Management  │   │  DuckDB SQL     │
                      └──────────────────┘   └────────────────┘
```

## 安装

```bash
git clone https://github.com/647133036/go-AxData.git
cd go-AxData
go build -o axdata-go .
```

Go 1.24+

## 快速开始

```bash
# 查看数据源列表
./axdata-go sources list

# 查看可用表和接口
./axdata-go data list

# 添加采集任务
./axdata-go collector add \
  --name "daily" \
  --source tencent \
  --interface stock_zh_a_hist_tx \
  --table daily \
  --params '{"codes":"000001.SZ,000002.SZ","period":"daily"}'

# 启动 API 服务
./axdata-go api serve --port 8080
```

## 数据源

| 数据源 | 类型 | 协议 | 接口数 | 说明 |
|--------|------|------|--------|------|
| **TDX**（通达信） | 行情 | TCP 7709 二进制 | 8 | 日线/分钟线、连板天梯、ST/停牌列表、题材强度排行；**需本地通达信客户端**，外网环境可能不可达 |
| **腾讯财经** | HTTP | HTTP API | 5 | 实时行情快照、个股/指数日K线、逐笔成交 |
| **新浪财经** | HTTP | HTTP API | 4 | 实时行情、日K/周K/月K/年K、板块排行；部分环境返回 403 |
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

### 采集任务

```bash
# 添加任务
./axdata-go collector add \
  --name "tencent-000001" \
  --source tencent \
  --interface stock_zh_a_hist_tx \
  --table daily \
  --params '{"codes":"000001.SZ","period":"daily"}'

# 查看任务列表
./axdata-go collector list

# 执行采集
./axdata-go collector run

# 查看任务状态
./axdata-go collector status
```

### 查询

```bash
# DuckDB SQL 查询
./axdata-go query "SELECT * FROM daily WHERE trade_date > '2026-01-01' LIMIT 10"

# API 查询
curl "http://localhost:8080/api/query?sql=SELECT * FROM daily LIMIT 10"
```

## 输出格式

| 格式 | 路径 | 说明 |
|------|------|------|
| Parquet | `$AXDATA_ROOT/data/<layer>/<table>.parquet` | 列式存储，适合分析 |
| DuckDB | 内存 | 通过 Querier 提供 SQL 查询 |

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AXDATA_ROOT` | `./axdata_data` | 数据根目录 |
| `AXDATA_TDX_HOSTS` | 内置服务器列表 | TDX 自定义主机列表，逗号分隔 `host:port` |

## 测试

```bash
# 全量测试
go test -v -count=1 ./core/... . -timeout 30s

# 单模块测试
go test -v -count=1 ./source-cninfo/...
go test -v -count=1 ./source-eastmoney/...
go test -v -count=1 ./source-cls/...
go test -v -count=1 ./source-tencent/...

# 集成测试
go test -v -count=1 -run TestIntegration .
```

当前 19 个包，160+ 测试函数，全部通过。

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
│   ├── collector/       # 采集任务系统
│   ├── config/          # 配置管理
│   ├── plugin/          # 插件系统
│   ├── query/           # DuckDB 查询器
│   ├── schema/          # 表结构定义（63 张表）
│   ├── source/          # Provider 注册表（99 个接口）
│   └── storage/         # Parquet 存储
├── source-*/            # 10 个数据源适配器（独立 Go module）
├── integration_test.go  # 集成测试
└── go.work              # Go workspace
```

## 许可证

MIT
