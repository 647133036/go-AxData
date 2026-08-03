# AxData-go

A股量化数据收集与分析平台。Go 语言版，兼容 Python 版 [AxData](https://github.com/axdata/axdata) 的数据模型和 API 规范。

## 架构

```
┌─────────────────┐    ┌───────────────────┐    ┌────────────────┐
│  Source Adapter │───▶│  ProviderRegistry  │───▶│  Schema Table  │
│  (10 sources)   │    │  (99 interfaces)   │    │  (59 tables)   │
│                 │    │  Field Mapping     │    │  Parquet/DB    │
└─────────────────┘    └──────────┬────────┘    └────────┬───────┘
                                   │                     │
                                   ▼                     ▼
                          ┌──────────────────┐  ┌────────────────┐
                          │  Collector       │  │  Querier       │
                          │  Task Management  │  │  DuckDB SQL     │
                          └──────────────────┘  └────────────────┘
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
# 列出所有数据源
./axdata-go sources list

# 查看所有支持的表和接口
./axdata-go data list

# 添加采集任务（以 mock 数据为例）
./axdata-go collector add --name "mock-daily" --source mock --interface daily --table daily

# 执行采集任务
./axdata-go collector run

# 查询数据（DuckDB SQL）
./axdata-go query "SELECT * FROM daily LIMIT 10"

# 启动 API 服务
./axdata-go api serve --port 8080
```

## 数据源

| 数据源 | 类型 | 协议 | 接口数 | 说明 |
|--------|------|------|--------|------|
| **TDX**（通达信） | HTTP | TCP 7709 二进制 | 8 | 日线/分钟线、连板天梯、ST/停牌列表，需要本地通达信客户端 |
| **腾讯财经** | HTTP | HTTP API | 5 | 实时行情、日K线、指数K线、逐笔成交 |
| **新浪财经** | HTTP | HTTP API | 4 | 实时行情、日/周/月/年K线、板块排行 |
| **东方财富** | HTTP | HTTP API | 15 | 实时行情、龙虎榜、融资融券、研报、涨跌停池、异动 |
| **财联社** | HTTP | HTTP API | 17 | 市场情绪、热门板块/概念/个股、涨停池、行业排行 |
| **开盘红** | HTTP | HTTP API | 10 | 板块排行、概念详情、连板天梯、市场复盘 |
| **同花顺** | HTTP | HTTP API | 1 | 人气榜 |
| **巨潮资讯** | HTTP | HTTP API | 32 | 公司档案、公告、分红、股东、股权质押、债券、基金持仓 |
| **i问财** | HTTP | HTTP API | 1 | 自然语言选股策略查询 |
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

# 执行任务
./axdata-go collector run

# 查看任务状态
./axdata-go collector status
```

### 查询

```bash
# DuckDB SQL 查询
./axdata-go query "SELECT * FROM daily WHERE trade_date > '2026-01-01'"

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
go test -v -count=1 ./core/... ./source-*/... .

# 单模块
go test -v -count=1 ./source-tencent/...

# 集成测试
go test -v -count=1 -run TestIntegration .
```

当前 19 个包，160 个测试函数，全部通过。

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
│   ├── schema/          # 表结构定义（59 张表）
│   ├── source/          # Provider 注册表（99 个接口）
│   └── storage/         # Parquet 存储
├── source-*/            # 10 个数据源适配器（独立 Go module）
├── integration_test.go  # 集成测试
└── go.work              # Go workspace
```

## 许可证

MIT
