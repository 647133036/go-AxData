# AxData-go

Go 语言版 AxData —— A股量化数据收集与分析平台，兼容 Python 版 [AxData](https://github.com/axdata/axdata) 的数据模型和 API 规范。

## 特性

- **10 个数据源适配器**：TDX（通达信）、东财、同花顺、新浪财经、腾讯财经、财联社、巨潮资讯、Wind、理杏仁、mock
- **99 个标准接口**：覆盖日线、分钟线、实时行情、板块、题材、龙虎榜、财报等全量数据
- **标准化 ETL 管道**：`Adapter → ProviderRegistry → FieldMapping → Schema → DuckDB/Parquet`
- **零外部 TDX SDK 依赖**：纯 Go TCP 协议实现通达信 7709 接口
- **多模块 Go workspace**：每个数据源独立模块，核心模块统一维护
- **完整测试覆盖**：单元测试、组件测试、集成测试

## 架构

```
┌─────────────┐     ┌──────────────────┐     ┌────────────────┐
│ Source      │     │ ProviderRegistry │     │ Schema Table   │
│ Adapters    │────▶│ (99 interfaces)  │────▶│ (59 tables)    │
│ (10 sources)│     │ Field Mapping    │     │ Parquet Output │
└─────────────┘     └──────────────────┘     └────────────────┘
                        │                          │
                        ▼                          ▼
                  ┌──────────────────┐     ┌────────────────┐
                  │   Collector      │     │    Querier     │
                  │   (Task System)  │     │   (DuckDB)     │
                  └──────────────────┘     └────────────────┘
```

### 核心模块

| 模块 | 路径 | 说明 |
|------|------|------|
| `core` | `./core` | 核心配置、schema、provider、collector、storage、query |
| `source-tdx` | `./source-tdx` | 通达信协议适配器（TCP 7709） |
| `source-tencent` | `./source-tencent` | 腾讯财经适配器 |
| `source-cninfo` | `./source-cninfo` | 巨潮资讯适配器 |
| `source-sina` | `./source-sina` | 新浪财经适配器 |
| `source-eastmoney` | `./source-eastmoney` | 东方财富适配器 |
| `source-cls` | `./source-cls` | 财联社适配器 |
| `source-kph` | `./source-kph` | 看盘汇适配器 |
| `source-mock` | `./source-mock` | Mock 测试适配器 |
| `source-ths` | `./source-ths` | 同花顺适配器 |
| `source-wencai` | `./source-wencai` | 文财适配器 |

## 安装

```bash
git clone https://github.com/647133036/go-AxData.git
cd go-AxData
go build -o axdata-go .
```

## 快速开始

```bash
# 查看数据源列表
./axdata-go sources list

# 查询可用表
./axdata-go data list

# 采集数据（需要网络）
./axdata-go collector add --name "daily" --source tdx --interface stock_kline_daily_tdx --table daily --codes 000001.SZ,000002.SZ

# 启动 API 服务
./axdata-go api serve --port 8080
```

## 配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `AXDATA_ROOT` | `./axdata_data` | 数据根目录 |
| `AXDATA_TDX_HOSTS` | 内置服务器列表 | TDX 自定义主机列表 |

## 数据源说明

### TDX（通达信）
- 协议：TCP 7709 二进制协议
- 实现：纯 Go 网络层，零外部 SDK 依赖
- 支持：日线、分钟线、实时行情、板块、题材

### 东方财富
- 接口：19 个标准接口
- 支持：实时行情、龙虎榜、融资融券、研究报告

### 新浪财经
- 接口：4 个标准接口
- 支持：日线、实时、分钟线、排行榜

### 腾讯财经
- 接口：5 个标准接口
- 支持：日线、实时、分钟线

### 财联社
- 接口：17 个标准接口
- 支持：情绪指标、风口板块、热点主题

### 巨潮资讯
- 接口：32 个标准接口
- 支持：公司公告、财报、债券、股本、分红

## 测试

```bash
# 运行所有测试
go test -v -count=1 ./core/... . -timeout 30s

# 运行单模块测试
go test -v -count=1 ./source-cninfo/...
go test -v -count=1 ./source-eastmoney/...
go test -v -count=1 ./source-cls/...
go test -v -count=1 ./source-tencent/...

# 运行集成测试
go test -v -count=1 -run TestIntegration .
```

## 开发

```bash
# 添加新数据源（以 mysource 为例）
mkdir source-mysource
cat > source-mysource/go.mod << 'EOF'
module github.com/electkismet/axdata-source-mysource
go 1.24
EOF

# 实现 Adapter 接口
cat > source-mysource/mysource.go << 'EOF'
package mysource

import "context"

type MySourceAdapter struct{}

func NewMySourceAdapter() *MySourceAdapter { return &MySourceAdapter{} }
func (a *MySourceAdapter) Name() string { return "mysource" }
func (a *MySourceAdapter) Description() string { return "MySource Adapter" }
func (a *MySourceAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
    // 实现数据获取逻辑
    return nil, nil
}
EOF

# 在 main.go 中注册
# source.Register(mysource.NewMySourceAdapter())
```

## 许可证

MIT
