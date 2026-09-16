# Requirements Document — Analyst Suite (行情/基本面/财报/估值/组合)

## Introduction

在现有 AxData-go 数据采集平台之上，补充一套面向个人投资者的**本地优先**分析能力：盯盘看行情并出带指标图表、梳理公司基本面、做财报前瞻与复盘、用 DCF 判断贵贱、检查组合分散度。所有计算在本机完成，数据优先读取本地 Parquet 缓存，缺失时回源并回写缓存。

## Glossary

- **System**：AxData-go 命令行程序（`axdata`）
- **Market Module**：行情与指标计算模块，位于 `core/market`、`core/indicator`、`core/chart`
- **Fundamental Module**：基本面梳理模块，位于 `core/fundamental`
- **Earnings Module**：财报前瞻与复盘模块，位于 `core/earnings`
- **Valuation Module**：DCF 估值模块，位于 `core/valuation`
- **Portfolio Module**：组合分散度分析模块，位于 `core/portfolio`
- **Local Cache**：`$AXDATA_ROOT/data/core/*.parquet` 中的本地历史数据
- **Security Code**：AxData 证券代码格式，形如 `000001.SZ`

---

## Requirement 1 — 技术指标计算引擎

**User Story:** AS 量化用户，I want 系统按标准公式计算常用技术指标，so that 我能在任何数据源行情上得到一致的指标数值。

#### Acceptance Criteria

1. THE System SHALL 支持计算移动平均线 MA，周期范围为 1 至 500。
2. THE System SHALL 支持计算指数移动平均 EMA，周期范围为 1 至 500。
3. THE System SHALL 支持计算 RSI，默认周期 14，采用 Wilder 平滑法。
4. THE System SHALL 支持计算 MACD，默认参数为快线 12、慢线 26、信号线 9。
5. THE System SHALL 支持计算 KDJ，默认参数为 9、3、3。
6. THE System SHALL 支持计算布林带 BOLL，默认周期 20、标准差倍数 2。
7. THE System SHALL 支持计算真实波幅 ATR，默认周期 14。
8. IF 序列长度小于指标所需的最小样本数，THE System SHALL 返回 `ErrInsufficientData` 错误并指明所需最小样本数。
9. THE System SHALL 对含 0 价格或缺失值的序列执行防御性处理，不产生 NaN 传播到最终输出。

---

## Requirement 2 — Stock Market Pro：盯行情与带指标图表

**User Story:** AS 看盘用户，I want 系统本地优先地返回实时行情并输出带 RSI/MACD 的图表，so that 我在终端里就能完成盯盘和形态判断。

#### Acceptance Criteria

1. WHEN 用户执行 `axdata market quote CODES`，THE System SHALL 返回每个代码的现价、涨跌幅、成交量、成交额、市盈率、市净率、总市值。
2. WHEN 用户执行 `axdata market watch CODES --interval 秒`，THE System SHALL 按间隔轮询刷新行情，直至收到终止信号。
3. IF 行情数据源请求失败，THE System SHALL 回退到次优数据源并输出降级提示，全部失败时返回明确错误。
4. WHEN 用户执行 `axdata market chart CODE`，THE System SHALL 优先从本地缓存读取日线，缺失时回源并写入本地缓存。
5. THE System SHALL 在图表中同时展示 K 线、MA5/MA10/MA20 均线与 RSI、MACD 副图。
6. THE System SHALL 支持 `--format ascii` 输出终端字符图，`--format svg` 输出单文件 SVG。
7. THE System SHALL 在图表下方输出最近 N 根 K 线的指标明细表，默认 N=10。

---

## Requirement 3 — Longbridge Fundamentals：公司底子梳理

**User Story:** AS 价值投资者，I want 系统快速梳理一家公司的财务报表、主营构成、行业地位与估值倍数，so that 我用几分钟能读懂公司底子。

#### Acceptance Criteria

1. WHEN 用户执行 `axdata fundamental CODE`，THE System SHALL 输出五个板块：公司概况、财务概况、主营构成、行业地位、估值倍数。
2. THE System SHALL 在财务概况中给出最近 4 个报告期的营业收入、归母净利润及同比增速。
3. THE System SHALL 在财务概况中给出毛利率、净利率、ROE、资产负债率。
4. THE System SHALL 在主营构成中列出业务分项的名称、收入、占比，按占比降序排列。
5. THE System SHALL 在行业地位中给出所属行业名称、行业平均市盈率与该公司市盈率、行业总市值排名。
6. THE System SHALL 在估值倍数中给出 PE(TTM)、PB、PS(TTM)、PEG、总市值、流通市值。
7. IF 任一板块数据不可用，THE System SHALL 输出 `N/A` 并附缺失原因，其他板块正常输出。
8. THE System SHALL 支持 `--format json` 输出结构化 JSON。

---

## Requirement 4 — Earnings Analysis：财报前瞻与复盘

**User Story:** AS 财报跟踪者，I want 系统给出财报前瞻和复盘总结，so that 我知道业绩是否符合预期以及指引是否变化。

#### Acceptance Criteria

1. WHEN 用户执行 `axdata earnings CODE`，THE System SHALL 输出三部分：业绩预告、业绩复盘、机构预期变化。
2. THE System SHALL 在业绩预告中列出预告期间、预告类型、预告净利润上下限或变动区间。
3. THE System SHALL 在业绩复盘中对比最近一期实际净利润与业绩预告区间，并判定 `超预期`、`符合预期`、`低于预期`、`无预告可比`。
4. THE System SHALL 在机构预期变化中给出最近评级、上次评级、评级变动与目标价区间。
5. THE System SHALL 生成不超过 5 条的要点总结，覆盖业绩方向、超预期的幅度、指引变化。
6. IF 该公司无业绩预告记录，THE System SHALL 明确输出"暂无业绩预告"并给出最近一期实际业绩。
7. THE System SHALL 支持 `--format json` 输出结构化 JSON。

---

## Requirement 5 — DCF Valuation：现金流折现估值

**User Story:** AS 估值用户，I want 系统用现金流折现模型算出内在价值，so that 我能判断当前股价的安全边际。

#### Acceptance Criteria

1. WHEN 用户执行 `axdata valuation dcf CODE`，THE System SHALL 采用 FCFF 两阶段模型计算内在价值。
2. THE System SHALL 显式预测期为 5 年，终值采用永续增长法。
3. THE System SHALL 支持用户覆盖预测增长率、WACC、永续增长率、净债务、股本数。
4. THE System SHALL 输出企业价值、股权价值、每股内在价值、当前价格、安全边际百分比。
5. THE System SHALL 输出 WACC 与永续增长率的敏感性矩阵，取值范围为 WACC ±2 个百分点、永续增长率 ±1 个百分点，步长 0.5 个百分点。
6. THE System SHALL 对负自由现金流序列给出提示，改用营运现金流作为代理并标注该替换。
7. IF 股本数或股价缺失，THE System SHALL 返回明确错误并指明缺失字段。
8. THE System SHALL 支持 `--format json` 输出结构化 JSON。

---

## Requirement 6 — Diversification：组合分散度分析

**User Story:** AS 组合管理者，I want 系统通过资产间相关性判断组合是否配置失衡，so that 我知道该怎么调整持仓。

#### Acceptance Criteria

1. WHEN 用户执行 `axdata portfolio analyze HOLDINGS`，THE System SHALL 接受 `CODE:WEIGHT` 逗号分隔的持仓输入，权重为百分比。
2. THE System SHALL 输出资产间的相关系数矩阵，样本窗口默认 250 个交易日。
3. THE System SHALL 计算组合波动率、各资产对组合风险的贡献度、集中度 HHI 指数。
4. THE System SHALL 计算分散化比率（组合波动率与加权平均个体波动率之比）。
5. IF 两个资产的相关系数大于 0.8，THE System SHALL 在报告中列出该高相关配对并提示冗余风险。
6. IF 单一资产权重超过 30%，THE System SHALL 提示集中度风险。
7. THE System SHALL 输出不超过 5 条再平衡建议。
8. THE System SHALL 支持 `--format json` 输出结构化 JSON。

---

## Requirement 7 — 数据接入扩展

**User Story:** AS 平台维护者，I want 新增财务与估值数据接口，so that 上述分析模块有稳定的数据来源。

#### Acceptance Criteria

1. THE System SHALL 新增东财财务报表接口 `eastmoney_financial_income`、`eastmoney_financial_balance`、`eastmoney_financial_cashflow`。
2. THE System SHALL 新增东财主营构成接口 `eastmoney_business_scope`。
3. THE System SHALL 新增东财业绩预告接口 `eastmoney_earnings_forecast`。
4. THE System SHALL 新增东财每日估值快照接口 `eastmoney_valuation_snapshot`。
5. THE System SHALL 为上述接口在 Schema Registry 中登记对应表结构与字段映射。
6. WHEN 新增接口请求成功，THE System SHALL 返回的记录字段名与登记表列名一致。
