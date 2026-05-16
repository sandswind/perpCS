# Perp Crisis Sandbox — 需求文档 (Requirements)

> 一句话定位：**一个链上 Perp 极限生存游戏。在内存里复刻 Hyperliquid 级别的 CLOB 体验，在历史灾难行情里把交易员"按在地上摩擦"，让用户练心态、练风控、练手速。**

---

## 1. 产品愿景

伴随 CLOB（链上中央限价订单簿）浪潮的崛起，新一代平台（Hyperliquid / Aster / Lighter / EdgeX）正以"零手续费让利、多链资金聚合、更隐私的执行环境"快速蚕食传统 L2 永续合约的份额。这些平台已经把"交易体验"做到了极致，但**没有人解决一个问题：交易员的心态训练**。

`Perp Crisis Sandbox` 不是又一个 DEX，而是一个**交易员的数字沙盘 / 抗压测试器**：

- **以游戏的方式**重放历史灾难（312、519、Luna 归零、FTX 爆雷、85 闪崩……）。
- **以 CLOB 级别的真实感**让用户感受到滑点、插针、流动性枯竭、ADL、预言机延迟。
- **以链上凭证**把"在地狱里活下来"这件事变成可炫耀、可携带的链上荣誉。

撮合、PnL、强平等**全部在沙盒内模拟**，不涉及真实成交，因此可以合法地放开"极端行情"的设计上限。

---

## 2. 竞品对标 (Reference Landscape)

| 平台 | 我们要学的 | 我们不做的 |
|---|---|---|
| **Hyperliquid** | 链上 CLOB 全订单簿可见、低延迟下单、HyperBFT 级别的确定性结算手感 | 不做真实清结算，撮合留在沙盒内 |
| **Aster (BNB Chain)** | 多链资金聚合、低门槛入场、积分制激励 | 不做实际跨链桥与流动性 |
| **Lighter (zk-Rollup)** | 零 Gas 体验、隐私执行环境、Maker 友好 | 不做 zk 证明，沙盒里直接给"零延迟无费用"体感 |
| **EdgeX** | 移动端友好、社交化交易、PnL 分享 | 不复制其 CEX-like 撮合，但借鉴 UX |

**差异化护城河**：竞品都在卷"如何让交易更顺滑"，我们卷"**如何让交易最难受、但用户还想再来一把**"。

---

## 3. 目标用户 (Personas)

1. **新人交易员 Alex**：刚入圈 6 个月，没经历过完整熊市，想在不亏真金白银的情况下"提前体验一次 312"。
2. **老韭菜 Bob**：经历过几轮牛熊，但仍然在重要节点情绪化操作，希望通过反复演练形成肌肉记忆。
3. **Quant / 策略开发者 Carol**：想验证自己的止损策略、仓位管理算法在极端行情下是否会被插针打穿。
4. **链上社交玩家 Dave**：不交易，但喜欢收集 SBT / 在 X 上炫耀通关战绩。

---

## 4. 核心用户故事 (User Stories)

### 4.1 入场（USDR 模型）
- 作为玩家，我希望**用钱包一键进入**（SIWE + Session Key）。
- 作为玩家，我完成反女巫验证后**一次性领取 10,000 USDR** 到钱包（自由转账）。
- 作为玩家，如果我钱包不够，可以**直接到 Uniswap 用 USDC/ETH 购买 USDR**，价格亲民。
- 作为玩家，我进入关卡前需要把 USDR **充值到 GameVault**，换取等量的游戏内余额；关卡结束后剩余余额可以**提回钱包**。
- 作为玩家，所有行情价格、订单簿、PnL、资金费率均以 **USDR 计价**（沙盒内 1 USDR ≡ 1 USD 历史价）。
- 作为玩家，我**通关 / 上榜**会从**奖池**额外获得 USDR 奖励，进入钱包。

### 4.2 选择关卡 / 难度
- 作为玩家，我可以选择三个**币对**之一：`BTC-EASY` (L1) / `BTC-MED` (L2) / `BTC-HARD` (L3)。
- 作为玩家，每个币对**独立运行**，有自己的订单簿、自己的玩家池、自己的难度参数。
- 作为玩家，我可以看到**关卡时长、关卡 KPI**（例：BTC-MED 中存活 60 分钟且净值 ≥ 入场金 80%）。
- 作为玩家，我能在每个币对里看到**当前历史原型**（"本场重放：2020/03/12 BTC 黑色星期四"）。

### 4.3 交易
- 作为玩家，我可以下 **市价单 / 限价单 / 止损单 / 止盈单**。
- 作为玩家，我可以选择 **逐仓 / 全仓**。
- 作为玩家，我看到的是**真实深度的订单簿**，市价吃单时按真实档位逐档成交并产生滑点。
- 作为玩家，我能看到**资金费率倒计时与历史费率曲线**。

### 4.4 危机
- 作为玩家，当我接近强平线，应该收到**清晰的视觉/音效告警**。
- 作为玩家，强平**按真实规则触发**（接管→市场卖出→保险基金→ADL）。
- 作为玩家，在 BTC-HARD 我接受**前端会模拟"拔网线"或卡顿**——这是设计的一部分；后端撮合不受影响，但前端会刻意制造体感障碍。

### 4.5 终局
- 作为玩家，关卡结束我能看到一份**复盘报告**：PnL 曲线、最大回撤、决策时间分布、情绪指标（取消单频率、改单频率）。
- 作为玩家，达成成就时**铸造一枚 SBT**（不可转让）作为链上荣誉。
- 作为玩家，我能把战绩**一键分享到 X / Farcaster**。

### 4.6 社交
- 作为玩家，我能看到**全局排行榜**（按币对 / 按周期）。
- 作为玩家，我能围观高分玩家的**交易回放**（类似 dota replay）。

### 4.7 排行榜与段位
- 作为新玩家 Alex，我希望进入游戏就能看到**多个排行榜**：通关速度榜、交易量榜、收益榜——给我多种"我可以追求的目标"。
- 作为老玩家 Bob，我希望即使不是当周冠军，**累计积分**也能让我看到自己段位在缓慢爬升（青铜→白银→黄金→铂金→钻石→传奇）。
- 作为玩家，我希望榜单**按币对（难度）分桶**——我在 BTC-EASY 拿不到前 10 也没关系，至少 BTC-EASY 榜首是我看得见的目标。
- 作为玩家，我希望**自成交、wash trade 不算量**——榜单要公正，否则刷量党会赶走真玩家。
- 作为玩家，我能看到**自己在每个榜的实时排名**，被超越时收到通知。
- 作为隐私敏感的玩家 Carol，我能选择**不公开上榜**（仍参与段位计算，但地址不在公开榜里）。
- 作为玩家，达到段位里程碑（如首次进 Gold）会**铸造 SBT**作为永久荣誉。

---

## 5. 功能需求 (Functional Requirements)

### FR-1 账户与资金 (Web3 Gate, USDR 模型)
- FR-1.1 钱包连接（EVM 优先，Phase 2 加 Solana / Sui）。
- FR-1.2 反女巫验证 (Sybil Gate)：玩家首次 claim 必须通过以下任一条件：
  - (a) Gitcoin Passport 分数 ≥ 阈值（推荐 15+）；
  - (b) 钱包链上年龄 ≥ 60 天且至少 1 笔历史交易；
  - (c) （可选）锁定 0.001 ETH 作为 anti-spam，关卡结束按完成度退还。
- FR-1.3 USDR Faucet 合约：通过反女巫的钱包可一次性 claim **10,000 USDR**（普通 ERC-20，可自由转账）。
- FR-1.4 GameVault：玩家进入关卡前调用 `deposit(amount, symbol)`，USDR 锁入合约，后端给该 (player, sessionId) 创建沙盒账户并记入等量余额。
- FR-1.5 GameVault.withdraw：关卡结束后，玩家拿后端阈值签名的 `Receipt` 调 `withdraw(receipt)`，提取剩余余额回钱包。
- FR-1.6 RewardPool：分级独立奖池架构，三个独立合约分别对应 BTC-EASY / BTC-MED / BTC-HARD（详见 FR-1A.5）。每个池子独立余额、独立多签、独立监控。
- FR-1.7 Uniswap 流动性：项目方在 USDR/USDC pool 注入初始流动性，**单价定为低面值**（如 1 USDC ≈ 1,000 USDR），让玩家能用极少的真金充值大量 USDR。
- FR-1.8 多链资金路由（Phase 2）：通过 LayerZero OFT 把 USDR 跨到 Arbitrum / Base / BNB Chain / Sui。

### FR-1A 代币经济 (USDR Tokenomics)
- FR-1A.1 USDR 是**标准 ERC-20**（无 transfer hook，无 soulbound 逻辑），符号 `USDR`，精度 18。
- FR-1A.2 总量硬顶 **100,000,000,000 USDR (1000 亿)**，**全部一次性 mint**，没有后续 mint 函数（mint 权限在部署完成后通过多签 renounce）。
- FR-1A.3 分配（一次性 mint 到对应金库合约）：

| 用途 | 数量 | 比例 | 说明 |
|---|---:|---:|---|
| Claim Faucet | 60,000,000,000 | 60% | 6 百万个钱包，每人 10,000 USDR 上限 |
| **Reward Pool L1** (BTC-EASY) | **10,000,000** | **0.01%** | 新手关卡奖池，独立合约 |
| **Reward Pool L2** (BTC-MED) | **50,000,000** | **0.05%** | 中级关卡奖池，独立合约 |
| **Reward Pool L3** (BTC-HARD) | **100,000,000** | **0.1%** | 高级关卡奖池，独立合约 |
| 团队 / 开发 | 10,000,000,000 | 10% | 4 年线性，1 年 cliff |
| 初始 DEX 流动性 | 5,000,000,000 | 5% | TGE 注入 USDR/USDC 池 |
| 生态预留 / 合作（含奖池补充储备） | 24,840,000,000 | 24.84% | 多签锁定，按提案释放 |

- FR-1A.4 Faucet 上限：每钱包终生 **claim 1 次**（合约 `claimed[address]` 记录）。

- FR-1A.5 **三层独立奖池架构 (Tiered Reward Pools)**

每个难度对应一个**独立的 RewardPool 合约**，独立多签、独立监控、独立预算：

| 池子 | 总额 (USDR) | 单局通关基础奖励 | 单周硬上限 | 理论容量 |
|---|---:|---:|---:|---|
| **PoolL1** (BTC-EASY) | 10,000,000 | 500 | 1% / 周 | ~20,000 通关名额 |
| **PoolL2** (BTC-MED) | 50,000,000 | 5,000 | 1% / 周 | ~10,000 通关名额 |
| **PoolL3** (BTC-HARD) | 100,000,000 | 50,000 | 1% / 周 | ~2,000 通关名额 |

> 现金价值参考（按 1 USDC ≈ 1,000 USDR 初始 LP 价格）：L1 ~$0.5、L2 ~$5、L3 ~$50。L3 单局奖励 50,000 USDR 远高于反女巫成本（≥ $5），但仍低于 BTC-HARD 实际通关难度的"心力成本"，激励真实玩家而非脚本党。

- FR-1A.5.1 通关判定（具体阈值后续 playtest 调参）：
  - L1 通关：净值 ≥ 入场金 80%；
  - L2 通关：净值 ≥ 入场金 50%；
  - L3 通关：净值 ≥ 入场金 10%（本质上是"少亏即赢"）。
- FR-1A.5.2 周榜额外奖励：每个池子单周可额外分配 ≤ 池子总额 1% 给周榜前 100 名（与单局通关奖励共享单周硬上限）。
- FR-1A.5.3 同一玩家同一关卡只能领一次奖（合约 `playedLevels[player][symbol][levelId]` 记录）。

- FR-1A.6 **奖池监控与安全机制（"监控通关率"的链上 + 链下落地）**

每个 RewardPool 合约必须实现：

- FR-1A.6.1 **链上余量公开**：`remaining()` 返回剩余 USDR，前端 Dashboard 实时显示。
- FR-1A.6.2 **单周硬上限 (`weeklyCap`)**：单 ISO 周内最大可发出 = 池子总额 × 1%。即使有人发现漏洞批量刷分，单周也只能拿走 1%，给运营留出处置时间。
- FR-1A.6.3 **余量阈值自动暂停**：余量低于 `pauseThreshold`（默认 5%）时合约自动拒绝新 `payout()`，强制走多签补充流程。
- FR-1A.6.4 **链下监控告警**（Reporting Svc）：
  - 单池单日发放 > 池子 0.5%：触发 P2 告警；
  - 单 IP 段 / 单 Gitcoin Passport 主体多次中奖：触发 P1 告警，自动冻结对应主体；
  - 通关率突变（24h 内通关率 > 历史均值 3σ）：触发 P0 告警，运营介入查 chaos config 是否被改动。
- FR-1A.6.5 **多签补充通道**：`refill(amount)` 函数可从生态储备转账补充，需 N-of-M 多签（建议 3-of-5）。补充事件链上公开。
- FR-1A.6.6 **每池独立的 Receipt 域名**：避免 L1 receipt 拿到 L3 的奖励——签名 EIP-712 domain 包含 `poolTier`。

- FR-1A.7 Uniswap 初始定价：USDR/USDC v3 池，定价**约 1 USDC = 1,000 USDR**，意味着 10,000 USDR 真金价值约 $10。"参与即送 10,000 USDR" 等同于 ~$10 的体验金，足够低不会被监管认定为博彩奖励，足够高让玩家有筹码下单。

- FR-1A.8 防 dump 措施：
  - 团队仓位锁仓 + 链上可验证；
  - 项目方 LP 锁仓 ≥ 1 年；
  - 单局奖励上限 50,000 USDR (~$50)；
  - 三个奖池独立 + 单周 1% 上限：最坏情况下单周市场冲击 ≤ 160 万 USDR (~$1.6k)，相对于 50 亿初始 LP 几乎不产生价格影响。

### FR-2 行情与数据源 (Chaos & Replay Engine)

#### FR-2.A 数据源 (Multi-Provider Data Sourcing)
- FR-2.A.1 抽象 `IDataProvider` 接口；初版接入 **Binance** 和 **Bitget**，后续可扩展（Bybit / OKX / Kraken / Tardis.dev 等）。
- FR-2.A.2 数据类型：每个 provider 必须能提供：
  - 1m / 1s K 线（历史回填）；
  - L2 订单簿快照 + delta（如有，用于真实深度回放；缺失时降级到合成深度）；
  - 逐笔成交 (aggTrade)；
  - 资金费率历史；
  - 指数价格历史。
- FR-2.A.3 Provider 适配层负责 **schema 归一化**：把不同交易所的字段、精度、时间戳格式统一成内部 `Tick` 结构。
- FR-2.A.4 关卡数据来源**多源对比**：同一历史时刻可同时拉取多个 provider 的数据，运营在后台选择"最有戏剧性"的版本作为该关卡的权威数据。
- FR-2.A.5 数据更新模式：
  - 历史数据：批处理 ETL，REST API 拉取归档 → 落 TimescaleDB；
  - 近期实时数据：WebSocket 订阅，落入 hot buffer，每天滚入 TimescaleDB。
- FR-2.A.6 数据完整性：每个关卡 dataset 落库时记录 (provider, symbol, time_range, sha256)，关卡启动时校验哈希，防止数据被篡改。
- FR-2.A.7 数据合规：仅使用各交易所**公开免费**的历史接口；如未来引入付费源（Tardis.dev / Coinalyze），加 license 字段在元数据。

#### FR-2.B 灾难关卡库 (Disaster Catalog)
覆盖至少以下 5 个原型 + 多 provider 多 symbol：

| 关卡 ID | 时间窗口 | 标的 | 主要 provider | 备选 provider |
|---|---|---|---|---|
| `D-312-BTC` | 2020-03-12 ~ 03-13 | BTC | Binance | Bitget |
| `D-519-BTC` | 2021-05-19 | BTC | Binance | Bitget |
| `D-LUNA` | 2022-05-09 ~ 05-12 | LUNA, UST | Binance | Bitget |
| `D-FTX` | 2022-11-08 ~ 11-10 | BTC, FTT, SOL | Binance | Bitget |
| `D-85` | 2024-08-05 | BTC, ETH | Bitget | Binance |

#### FR-2.C 回放与混沌 (Replay & Chaos)
- FR-2.C.1 回放速率：1x / 2x / 4x（仅 BTC-EASY 允许慢放教学）。
- FR-2.C.2 后端 Chaos Monkey **只负责行情层**（影响撮合的真实因素）：
  - 插针生成器（向下 / 向上 wick）；
  - 流动性枯竭（按比例 shrink 订单簿深度）；
  - 预言机延迟（mark price 滞后于 last price N 秒）；
  - 资金费率炸弹（每小时 ±1%）。
- FR-2.C.3 **客户端 Chaos**（仅影响体感、不改变成交结果）由前端实现：
  - UI 假死（Connecting…）；
  - 模拟 RPC 拥堵（人为延迟下单 ack 显示）；
  - 屏幕抖动 / 闪屏 / 音效；
  - 行情图表回退跳变。
- FR-2.C.4 后端**只下发 chaos 配置参数**（per symbol）给客户端，客户端根据参数自行决定何时触发何种"恶意效果"。后端不再做任何 UI 干预，撮合保持纯净、确定。

### FR-3 撮合 (Match Engine)
- FR-3.1 全内存 CLOB，支持 Limit / Market / Stop / Take-Profit / Reduce-Only / Post-Only。
- FR-3.2 价格-时间优先 (Price-Time Priority)。
- FR-3.3 用户单与历史回放挂单**共池撮合**（用户的 Limit 单可以被历史 Taker 吃掉，反之亦然）。
- FR-3.4 真实滑点：市价单按订单簿逐档吃。
- FR-3.5 IOC / FOK / GTC 时效。
- FR-3.6 **多 symbol 隔离**：每个币对（BTC-EASY / BTC-MED / BTC-HARD）有**独立的订单簿、独立的撮合 goroutine、独立的事件流**；同一 symbol 内所有玩家共享 book，跨 symbol 完全隔离。

### FR-4 多 Symbol 难度系统 (Difficulty as Symbols)

| Symbol | 难度 | 历史关卡池 | 杠杆上限 | 每局门票 (USDR) | 关卡时长 |
|---|---|---|---:|---:|---:|
| `BTC-EASY` | L1 新手试炼 | D-519-BTC | 10x | 100 | 30 min |
| `BTC-MED` | L2 心跳过速 | D-312-BTC, D-LUNA | 25x | 500 | 60 min |
| `BTC-HARD` | L3 精神崩溃 | D-FTX, D-85, 合成虚构剧本 | 50x | 1,000 | 60 min |

- FR-4.1 玩家通过 `GameVault.deposit(amount, symbol)` 进入对应难度，amount 至少等于该 symbol 的最小门票。
- FR-4.2 每个 symbol 有自己独立的 chaos config，由后端配置中心 (Postgres `chaos_config` 表) 管理，可热更新。
- FR-4.3 同一玩家**可同时参与多个 symbol**，但每个 symbol 内同一关卡只能玩一次（合约 `playedLevels[player][symbol][levelId]`）。

### FR-5 持仓与 PnL (Account State Machine)
- FR-5.1 全仓 (Cross) / 逐仓 (Isolated) 保证金。
- FR-5.2 杠杆范围：见 FR-4 表（高难度反而开放高杠杆，鼓励作死）。
- FR-5.3 实时未实现 PnL、维持保证金率、可用余额计算。
- FR-5.4 资金费率结算（按历史真实费率 + 难度加成）。

### FR-6 风控与强平 (Crisis Liquidation)
- FR-6.1 维持保证金率 (MMR) 触发接管。
- FR-6.2 强平流程：接管 → 市场出清 → 保险基金兜底 → ADL（自动减仓盈利对手方，仅 BTC-HARD 启用）。
- FR-6.3 穿仓模拟（仅 BTC-HARD）：保险基金耗尽，账户净值变负，向用户展示"穿仓 -X USDR"（仅展示，不会真的扣链上资金）。

### FR-7 战绩 / 社交
- FR-7.1 复盘报告（PnL 曲线、最大回撤、操作时间线、情绪指标）。
- FR-7.2 SBT 成就体系（"312 生还者"、"抗爆仓王者"、"插针之神"等）。
- FR-7.3 排行榜（详见 FR-7A）。
- FR-7.4 回放分享链接（无需登录即可观看）。

### FR-7A 排行榜 (Leaderboards)

> 设计原则：**3 个维度正交**（互不替代）、**按 symbol 分桶**（难度天然分级）、**周/全时窗口**（兼顾新鲜与累积）、**段位累计积分**（长期目标感）。

#### FR-7A.1 三个核心维度 (Boards)

| 维度 ID | 名称 | 排序键 | 反作弊规则 |
|---|---|---|---|
| **`speedrun`** | 通关速度榜 (First-Clear) | `clearTimeMs` 升序（最快者第一） | 仅记**首次通关**该 levelId 的时间；后续重玩不上此榜 |
| **`volume`** | 交易量榜 (Most Traded) | `cumNotional` 降序（最大者第一） | 自成交（self-match）不计入；wash 检测：单边持仓 < 5% 时长则不计；重复秒级反向开平的成交按 50% 折算 |
| **`pnl`** | 收益榜 (Highest PnL) | `realizedPnL` 降序（绝对 USDR 收益） | 只统计 `withdrawAmount - depositAmount`；ADL 被减仓获得的 PnL 不计；穿仓负值不入榜 |

> 三个榜**完全独立打榜**——同一玩家同一关卡可同时上 3 个榜，但**不会因为另一个榜分高而被挤掉**。这避免维度互相替代。

#### FR-7A.2 排行榜 × Symbol 分桶

每个维度按 symbol 独立维护：

```
leaderboard_keys = {speedrun, volume, pnl} × {BTC-EASY, BTC-MED, BTC-HARD} = 9 个独立榜单
```

外加 1 个**总榜**（cross-symbol，按段位积分排序，详见 FR-7A.4）。

#### FR-7A.3 时间窗口 (Time Windows)

每个 (维度, symbol) 同时维护 3 个窗口：

| 窗口 | Key | 重置 | 用途 |
|---|---|---|---|
| `daily` | `lb:{board}:{symbol}:d:{YYYY-MM-DD}` | 每日 00:00 UTC | 新鲜度，鼓励每日活跃 |
| `weekly` | `lb:{board}:{symbol}:w:{ISO_WEEK}` | 每周一 00:00 UTC | 配合 RewardPool 周发奖 |
| `alltime` | `lb:{board}:{symbol}:all` | 永不重置 | 历史成就，进 SBT |

**奖励发放**：
- **周榜前 100** 进入 `RewardPoolL{1|2|3}` 的周发奖名单；具体分配比例：
  - 第 1 名：**15%**
  - 第 2 – 10 名：每名 **4%**（共 9 × 4% = 36%）
  - 第 11 – 100 名：每名 **0.5%**（共 90 × 0.5% = 45%）
  - 合计：15 + 36 + 45 = **96%**（剩余 4% 沉淀，作为下周奖池缓冲，避免分配尾数误差）
  - 总和 ≤ 该周该 symbol 的奖池预算（受 weeklyCap 约束）。
- **日榜**：仅展示，不发奖（避免日活作弊）。
- **全时榜**：达到 Top 10 / 100 / 1000 触发对应 SBT 成就。

#### FR-7A.4 段位系统 (Tier / Rating)

> 段位是一个**累计积分**，跨 symbol、跨维度合并，给玩家一个长期目标。

**段位积分公式**（每场关卡结束后计算）：

```
sessionScore = base[symbol] × 通关系数 × 排名加成

  base[symbol]:
    BTC-EASY  = 10
    BTC-MED   = 50
    BTC-HARD  = 200

  通关系数:
    finalEquity ≥ 入场金 100%   → 1.0
    finalEquity ≥ 入场金 50%    → 0.5
    finalEquity ≥ 入场金 10%    → 0.2
    finalEquity <  入场金 10%   → 0.0

  排名加成 (基于 weekly 任一榜单):
    本周任一 weekly 榜 Top 1     → ×3
    本周任一 weekly 榜 Top 10    → ×2
    本周任一 weekly 榜 Top 100   → ×1.5
    其他                         → ×1.0
```

**段位等级**：

| 段位 | 累计积分阈值 | 标识 | 特权 |
|---|---:|---|---|
| **Bronze** (青铜) | 0 – 99 | 🥉 | — |
| **Silver** (白银) | 100 – 499 | 🥈 | 显示银色昵称边框 |
| **Gold** (黄金) | 500 – 1,999 | 🥇 | 金色边框 + 复盘高亮 |
| **Platinum** (铂金) | 2,000 – 9,999 | 💠 | 解锁回放快进 8x |
| **Diamond** (钻石) | 10,000 – 49,999 | 💎 | 钻石专属边框 + 复盘高级指标 |
| **Legend** (传奇) | 50,000+ | 👑 | 总榜 Top 1000 永久铸 SBT |

> **段位策略：初版只升不降**（积分累计），便于老玩家保留荣誉。
> 上线 6 个月后根据玩家分布数据复盘——如果 Legend 段位含金量被稀释，再决定是否引入"积分衰减"或"近 30 天活跃段位"等机制。

#### FR-7A.5 反作弊与公平规则

- **FR-7A.5.1 自成交不算量**：同一玩家同时挂 buy + sell 单互吃自己，撮合允许（不能搞特例破坏 CLOB），但 `volume` 榜单计算时**双方都是自己 → 计 0**。
- **FR-7A.5.2 Wash trade 检测**：60 秒内同 symbol 累计净持仓时长 < 总时长 5%（即频繁开平、几乎不持仓），该期间的成交量按 50% 折算。**初版采用此阈值，上线后根据实际数据观察是否误伤合规高频策略，再调整阈值或折算系数。**
- **FR-7A.5.3 同钱包多 session 累加 vs 取最优**：
  - `speedrun`：取该钱包对该 levelId 的**首次通关**记录（终生只能上一次）；
  - `volume`：单 session 取最大值（不跨 session 累加，避免反复入场刷量）；
  - `pnl`：单 session 取最大值（同上）；
  - 段位积分：跨 session 累加，但单关卡 levelId 只算一次最高分。
- **FR-7A.5.4 反女巫绑定**：上榜要求该钱包通过过 Faucet 反女巫验证（Gitcoin Passport 或链上年龄）。匿名钱包不上榜。
- **FR-7A.5.5 Receipt 校验**：榜单写入由 Reporting Svc 在关卡结算时基于 `eventsHash` 触发，不接受客户端直接提交分数。

#### FR-7A.6 排行榜 API

- `GET /leaderboard/{board}/{symbol}/{window}?limit=100&offset=0` → 返回 Top N
- `GET /leaderboard/rank/{address}` → 返回该地址在 9 个 (board, symbol) × 3 windows = 27 个榜里的排名
- `GET /leaderboard/tier/{address}` → 返回段位、累计积分、距离下一段位差距
- `GET /leaderboard/global/tier?limit=100` → 总榜（按累计积分跨 symbol 排序）
- WS topic `leaderboard.{symbol}.{board}.{window}` → 排名变化推送（用于"你刚被超越了"提醒）

#### FR-7A.7 数据可见性

- 上榜地址默认显示**前 6 位 + 后 4 位**（`0xabcd…1234`）；
- 玩家可在个人中心绑定**匿名昵称**（链下，可改）；
- 复盘 / 战绩可在玩家选择下**公开分享**给非玩家（生成短链）；
- 玩家可主动选择**退出公开榜单**（`optOutPublic = true`），但仍参与段位计算（不公开但可自查）。

---

## 6. 非功能需求 (Non-Functional Requirements)

| 维度 | 指标 |
|---|---|
| **下单延迟** | p99 ≤ 30ms（撮合内核内）；前端到撮合 p99 ≤ 150ms（不含客户端 chaos 模拟的延迟） |
| **行情推送** | WebSocket，订单簿 snapshot + delta，≤ 100ms |
| **并发** | 单 symbol ≥ 1k 同时在线玩家；3 symbols 并行运行 |
| **撮合吞吐** | 单引擎 ≥ 50k orders/sec |
| **可重放性** | 同一 (symbol, sessionId, chaos seed) 必须产生**完全确定性**的市场 |
| **公平性** | 同一 symbol 内所有玩家看到的行情完全一致；chaos seed 在关卡开始时锁定 |
| **服务端纯净** | 后端撮合**完全确定**，不做任何 UI 干预（拔网线、抖动等全在前端） |
| **可观测** | 每笔订单 / 每次强平 / 每次 chaos 注入都落审计日志 |

---

## 7. MVP 切片 (Scope)

**Phase 0 – Playable Demo (4 周)**
- 单 symbol (`BTC-MED`)，单关卡 (D-312-BTC)；
- Web 钱包连接 + Faucet claim；
- GameVault 充提；
- 内存撮合 + 强平 + 复盘页；
- 数据源接入 Binance（单源）。

**Phase 1 – Public Beta (8 周)**
- 三个 symbol 全开 (`BTC-EASY` / `BTC-MED` / `BTC-HARD`)；
- Bitget 第二数据源 + 多源对比；
- 链上 RewardPool + SurvivorBadge SBT；
- Uniswap LP 注入；
- 排行榜、回放分享；
- 客户端 chaos 引擎（UI 假死、抖动、音效）。

**Phase 2 – Multi-chain & Social (12 周+)**
- 多链入场（Arbitrum / Base / BNB / Sui）；
- 更多数据源（Bybit / OKX）；
- API 模式（Quant 用户可以挂自动策略测试）；
- 公会 / 锦标赛模式；
- 移动端。

---

## 8. 风险与合规

- **不是真实交易**：所有撮合发生在沙盒，玩家充值的 USDR 可在关卡结束后提回，**门票本身不消失**（只是赢输）。UI 必须显著展示 "Simulation Only / Not Financial Advice / USDR is a utility token"。
- **USDR 不锚定法币**：白皮书 / 前端必须明确声明 "USDR is **not** a stablecoin and is **not** redeemable for USD"。沙盒内 1 USDR ≡ 1 USD 仅是**显示口径**，链下市场价由 Uniswap 决定。
- **反女巫**：claim faucet 通过 Gitcoin Passport / 链上年龄 / 小额 ETH 锁三选一。
- **地理限制**：屏蔽美国、中国大陆、OFAC 制裁地区 IP；claim 前弹合规免责。
- **未成年保护**：钱包连接时弹年龄确认。
- **历史数据合规**：使用各交易所公开免费历史接口；记录每条数据的 provider 与时间戳，便于审计。
- **链上数据隐私**：玩家地址会出现在排行榜，提供"匿名昵称"开关。
- **代币合规**：Earn 奖励来自固定的 RewardPool（10M USDR）而非每局 mint，避免被认定为"工作量证券"；如有疑虑，初版仅在 testnet 发币，主网推迟到法律审核完成。
- **客户端 chaos 免责**：BTC-HARD 入场前必须弹窗"本难度会模拟网络故障与界面卡顿，是设计的一部分，请确认参与"，玩家点击"我理解"才能进入。
