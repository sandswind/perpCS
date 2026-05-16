# Perp Crisis Sandbox — 任务拆分与版本规划 (Tasks & Roadmap)

> 本文档承接 `requirements.md` 与 `design.md`。
> **核心理念**：**先落地，再迭代**。每个版本都是一个**可演示、可验收**的最小可运行产物（MVR, Minimum Viable Release），而不是"开发了一堆模块但跑不起来"。
> 砍掉所有"为了完美一次到位"的诱惑，每 3–5 天交付一个能演示的东西。

---

## 0. 版本路线图 (Roadmap at a Glance)

```
v0.1  ───►  v0.2  ───►  v0.3  ───►  v0.4  ───►  v0.5  ───►  v0.6  ═══►  v1.x  ───►  v2.x
撮合骨架    CLI下单    Web交易    混沌+强平   链上充提    完整闭环   Phase 1   Phase 2
2~3 d       2~3 d       3~4 d      3~4 d       3~4 d       3~4 d
                                                              │
                                                              ▼
                                                        ★ MVP 完成 ★
                                                          (4 周内)
```

| 版本 | 名称 | 核心能力 | 演示场景 | 工期 | 累计工期 |
|---|---|---|---|---:|---:|
| **v0.1** | Replay Engine | 历史数据 + 内存撮合 + 终端 inspector | 开发者跑 `go run cmd/replay`，看 D-312 订单簿在终端流动 | 2~3 d | 3 d |
| **v0.2** | CLI Trader | + 单玩家命令行下单 | CLI 下限价/市价单，看成交、滑点、持仓 | 2~3 d | 6 d |
| **v0.3** | Web Trader | + Web UI + K 线 + WS 实时刷 | 浏览器登录（mock JWT），下单看仓 | 3~4 d | 10 d |
| **v0.4** | Chaos & Liquidation | + 服务端混沌（插针/深度/mark滞后）+ 强平 | 玩家开 10x 多单在 312 暴跌中被插针强平 | 3~4 d | 14 d |
| **v0.5** | On-chain Entry | + Sepolia 合约 + 钱包连接 + Faucet + Deposit | MetaMask 连接 → claim 10k USDR → deposit 500 USDR 进沙盒 | 3~4 d | 18 d |
| **v0.6** | E2E MVP ★ | + Receipt 签名 + Withdraw + 复盘页 | 完整闭环：deposit → 交易 → 强平/平仓 → withdraw 拿回 USDR + 复盘 | 3~4 d | **22 d ≈ 4周** |
| v1.0 | Multi-Symbol | + BTC-EASY / BTC-HARD | 玩家可选三难度币对 | 5 d | |
| v1.1 | Multi-Provider | + Bitget + 多源 diff | 数据源容灾 + 关卡多版本 | 4 d | |
| v1.2 | Reward Pools | + 三层奖池 + 周发奖 + SBT | 通关后链上领奖 + 铸 SBT | 6 d | |
| v1.3 | Leaderboard | + 9 榜 + 段位系统 | 玩家看到自己在 27 个排名里的位置 | 5 d | |
| v1.4 | Sybil & DEX | + 反女巫 + Uniswap LP + 多签 | 主网级别合规 | 6 d | |
| v1.5 | Client Chaos FX | + UI 假死 + 抖动 + 音效 | BTC-HARD 真正的"心跳过速"体验 | 4 d | |
| v2.x | Multi-chain / API | LayerZero OFT / Quant 接口 / 移动端 | — | — | |

> **设计原则**：
> 1. **每个 vX.Y 必须能演示** —— 哪怕只是终端日志，也要有可见输出；
> 2. **每个版本可独立 deploy / rollback**，不要"半拉子"状态；
> 3. **测试先行**：每加一个版本，先补该版本对应的端到端测试，再加下一个；
> 4. **MVP = v0.6**：后续 v1.x 全部按需求文档 Phase 1 范围落地，节奏不变。

---

## 1. 整体 In Scope / Out of Scope（MVP，对齐 requirements.md §7 Phase 0）

### 1.1 MVP (v0.1–v0.6) In Scope

| 维度 | 决定 |
|---|---|
| Symbol | **仅 1 个**：`BTC-MED` |
| 关卡 | **仅 1 个**：`D-312-BTC`（2020-03-12 BTC 黑色星期四） |
| 数据源 | **仅 Binance** |
| 链 | **Arbitrum Sepolia** testnet |
| 钱包 | EVM only，仅 MetaMask |
| 后端 chaos | L2 中度（插针 + 深度收缩 + mark 滞后） |
| 撮合 | 内存 CLOB、价格-时间优先、IOC/GTC、逐仓保证金 |
| 强平 | 接管 + 市价出清 + 保险基金兜底（不做 ADL/穿仓） |

### 1.2 MVP Out of Scope（明确推迟到 v1.x）

❌ BTC-EASY / BTC-HARD（v1.0）
❌ Bitget 第二数据源（v1.1）
❌ 三层 RewardPool 合约 + 周发奖（v1.2）
❌ 排行榜 + 段位（v1.3）
❌ SurvivorBadge SBT（v1.2）
❌ Uniswap V3 LP + 多签（v1.4）
❌ 反女巫 Gitcoin Passport（v1.4）
❌ 客户端 Chaos FX（UI 假死、抖动、音效）（v1.5）
❌ ADL / 穿仓模拟（v1.0 BTC-HARD 才有）
❌ Account Abstraction / Session Key（v1.4）
❌ 全仓保证金（仅做逐仓）
❌ 多链 / 移动端（v2.x）

---

## 2. 团队配置假设

| 角色 | 缩写 | 职责 |
|---|---|---|
| 合约 | **SC** | Solidity / Foundry / Sepolia |
| 后端 (Go) | **BE** | 撮合 + API + 数据接入（含 1 人偏数据） |
| 前端 (TS) | **FE** | Next.js + Wagmi + Chart |
| DevOps | **OPS** | CI / Docker / 测试网 / 监控 |

> **一人项目**：按依赖图串行，时间 ×1.5；**3~4 人**：按下面节奏并行。

---

## 3. 版本详情与任务清单

每个版本都包含：① 用户故事 → ② 任务清单 → ③ 验收标准（DoD）→ ④ 演示脚本。

---

### 🔧 v0.1 — Replay Engine（撮合骨架）

> **目标**：让 D-312 的历史数据能在内存里"流动"，订单簿随之变化。**不连数据库、不连前端、不连链**。这是整个系统的"心脏"，先确保它能跳。

#### 用户故事
作为开发者，我执行 `go run cmd/replay --level=D-312-BTC --speed=1x`，能在终端看到订单簿每秒刷新（imitating chaos clock），输出 `events.jsonl` 文件包含完整成交流。

#### 任务清单

| # | 任务 | Owner | 估算 |
|---|---|---|---|
| 0.1.1 | Go monorepo 脚手架 + Makefile | OPS | 0.3 d |
| 0.1.2 | `IDataProvider` 接口 + Mock provider（生成假 K 线） | BE | 0.5 d |
| 0.1.3 | Binance Adapter 最简版（仅 `FetchKlines` + aggTrades） | BE | 1.5 d |
| 0.1.4 | `OrderBook` 数据结构（跳表 + FIFO 链表）+ 单元测试 | BE | 2 d |
| 0.1.5 | `MarketActor` 最简版（只接 replay 单 + 自撮合，不接用户单） | BE | 1 d |
| 0.1.6 | CLI inspector：定时打印 best bid/ask + last trade | BE | 0.3 d |
| 0.1.7 | 事件输出 `events.jsonl` + sha256 校验 | BE | 0.4 d |

#### DoD
- ✅ `go test ./...` 通过，OrderBook 覆盖率 ≥ 80%
- ✅ OrderBook benchmark：单 goroutine ≥ 50k orders/sec
- ✅ 跑 D-312 完整 24 小时数据，终端能看到价格从 ~$7,900 跌到 ~$3,800
- ✅ 同 seed 跑 2 次，`events.jsonl` 的 sha256 完全一致（确定性验证）
- ⚠️ **风险检查点**：如果 Binance 拿不到 D-312 历史 aggTrades，立即启动 depth_synth fallback（推迟到 v0.4）

#### 演示脚本
```bash
$ make replay LEVEL=D-312-BTC SPEED=10x
[chaos clock 2020-03-12 02:00:00] BTC-MED bid 7892.30 / ask 7893.10  spread 0.80
[chaos clock 2020-03-12 02:30:00] BTC-MED bid 7820.00 / ask 7821.50  ↓72.30
[chaos clock 2020-03-12 03:00:00] BTC-MED bid 6500.00 / ask 6505.00  ↓1320.00 ← 雪崩开始
...
[done] events written to ./out/events-D312-1234.jsonl  sha256: a3f9...
```

---

### 💻 v0.2 — CLI Trader（命令行交易）

> **目标**：让开发者能用 CLI 下单与 v0.1 的撮合内核交互，看到滑点 / 持仓 / PnL。**仍无前端、无数据库、无链**。

#### 用户故事
作为开发者，我启动 replay 后，在另一个终端运行 `go run cmd/cli`，输入 `buy market 0.1 BTC-MED`，看到成交、持仓、未实现 PnL；下平仓单后看 realized PnL。

#### 任务清单

| # | 任务 | Owner | 估算 |
|---|---|---|---|
| 0.2.1 | `MarketActor` 加 `OrderQueue` 接收用户单（线程安全） | BE | 1 d |
| 0.2.2 | `AccountStateMachine` 简化版：markToMarket + uPnL（不强平） | BE | 1 d |
| 0.2.3 | HTTP `POST /orders` / `DELETE /orders/:id` / `GET /account` | BE | 1 d |
| 0.2.4 | 内存 session（无 sessionId 持久化） | BE | 0.3 d |
| 0.2.5 | CLI client：交互式 REPL（rdcli 风格） | BE | 1 d |

#### DoD
- ✅ CLI 下限价单挂在 book 上，能被 replay 单吃到
- ✅ CLI 下市价单按订单簿逐档吃，滑点符合预期
- ✅ 持仓实时更新 uPnL 随 mark 变化
- ✅ 撤单成功；重复撤单返回 `ALREADY_CANCELLED`

#### 演示脚本
```
$ go run cmd/cli
> connect localhost:8080 BTC-MED
session created, balance 10000 USDR

> buy limit 0.5 6000.00
order#1 placed (limit buy 0.5 @ 6000.00)

[fill] order#1 filled 0.5 @ 6000.00, cost 3000.00 USDR
position: long 0.5 @ 6000.00, uPnL +0.00

[mark update] mark 5800.00, uPnL -100.00

> close
[fill] sell market 0.5 @ 5798.50 (slippage -1.50)
position: closed, realized -100.75 USDR
```

---

### 🌐 v0.3 — Web Trader（浏览器交易）

> **目标**：把 v0.2 的能力搬到浏览器。**仍无链**——身份用 mock JWT 模拟"已登录玩家"。

#### 用户故事
作为玩家（mock 身份），我打开 `localhost:3000`，看到 K 线图 + 实时订单簿 + 下单面板，能下单 / 看持仓 / 平仓。

#### 任务清单

| # | 任务 | Owner | 估算 |
|---|---|---|---|
| 0.3.1 | Next.js 14 脚手架 + Tailwind + Zustand | FE | 0.5 d |
| 0.3.2 | WebSocket `/ws/market/:symbol` fanout（订单簿 snapshot + delta） | BE | 1 d |
| 0.3.3 | WebSocket `/ws/account/:sid` 推持仓/订单/fill | BE | 0.5 d |
| 0.3.4 | 交易页 3 栏布局：K 线（Lightweight Charts）+ 订单簿 + 订单输入 | FE | 2 d |
| 0.3.5 | 订单簿前端组件（增量合并、20 档买卖盘） | FE | 0.7 d |
| 0.3.6 | 下单 / 撤单 / 持仓 / 历史成交 UI + REST 调用 | FE | 1 d |
| 0.3.7 | docker-compose 一键起前后端 | OPS | 0.3 d |

#### DoD
- ✅ K 线图按 chaos clock 实时刷新（≤ 100ms）
- ✅ 订单簿 20 档买卖盘 + spread 高亮
- ✅ 下单按钮点击后，订单簿和持仓在 200ms 内可见反馈
- ✅ 50 个浏览器 tab 并发不卡
- ✅ 关闭 WS 后自动重连 + 补 snapshot

#### 演示脚本
1. `docker compose up`
2. 浏览器打开 `localhost:3000`，自动登录 mock 用户 `0xdead...beef`
3. 选关卡 BTC-MED → 进入交易页
4. 看 K 线从 $7900 缓慢下跌
5. 下市价多单 0.1 BTC → 持仓出现
6. 等几分钟看 uPnL 随 312 暴跌变红
7. 平仓收手 → 复盘卡片显示 PnL

---

### 💥 v0.4 — Chaos & Liquidation（混沌引擎 + 强平）

> **目标**：让 312 真的"恐怖"。加服务端混沌（插针、深度收缩、mark 滞后）+ 强平瀑布。**这是产品最有差异化的版本**。

#### 用户故事
作为玩家，我开 10 倍杠杆做多，在 312 关卡里被插针打穿强平线，眼睁睁看着保证金归零。

#### 任务清单

| # | 任务 | Owner | 估算 |
|---|---|---|---|
| 0.4.1 | `ChaosEngine` 完整实现：插针 + 深度收缩 + mark 滞后（per-tick 确定性） | BE | 2 d |
| 0.4.2 | `chaos_config` Postgres 表 + seed BTC-MED 参数 | BE | 0.3 d |
| 0.4.3 | TimescaleDB hypertable + ETL 把 D-312 落库（替换 v0.1 的 in-memory） | BE | 1 d |
| 0.4.4 | 强平瀑布：接管 → 市价出清 → 保险基金接盘（无 ADL） | BE | 2 d |
| 0.4.5 | 资金费率结算（每模拟 8h 一次） | BE | 0.5 d |
| 0.4.6 | 不变量自检：`Σ(玩家净值) + 保险基金 = 累计入场金` | BE | 0.3 d |
| 0.4.7 | 前端：强平警告 banner（marginRatio < 1.5x MMR 时变红 + 心跳音） | FE | 0.5 d |
| 0.4.8 | 前端：强平动画（"你被强平了"全屏遮罩） | FE | 0.5 d |
| 0.4.9 | depth_synth fallback（如果 v0.1 没做） | BE | 1 d（按需） |

#### DoD
- ✅ 同 (sessionId, seed) 跑 2 次，事件流 byte-identical（**核心确定性回归**）
- ✅ 312 关卡的插针在 K 线上清晰可见（向下 wick > 5%）
- ✅ 10x 杠杆开多 + 312 大跌 → 必然强平
- ✅ 不变量在每 tick 都成立（自动 panic 如果违反）
- ✅ 50 玩家共享 BTC-MED book 不崩，撮合 p99 ≤ 50ms

#### 演示脚本
1. v0.3 基础上重启
2. 多个浏览器 tab 模拟 5 个玩家，全部 10x 多单
3. 看 312 暴跌中陆续强平
4. 看后端日志：保险基金接盘记录
5. 关卡结束后看不变量自检日志：✅ 通过

---

### 🔗 v0.5 — On-chain Entry（链上接入）

> **目标**：把"用 MetaMask 连接 → 领 USDR → 充值进沙盒"这条路打通。**还没法 withdraw**（v0.6 做）。

#### 用户故事
作为玩家，我用 MetaMask 连 Sepolia → 一键领取 10,000 USDR → 选 BTC-MED 关卡 → 充值 500 USDR → 进入沙盒交易（沿用 v0.4 的全部能力）。

#### 任务清单

| # | 任务 | Owner | 估算 |
|---|---|---|---|
| 0.5.1 | Foundry 脚手架 + OZ + slither | SC | 0.3 d |
| 0.5.2 | `USDR.sol`（标准 ERC-20，1000 亿，5 vault 一次性 mint） | SC | 0.5 d |
| 0.5.3 | `USDRFaucet.sol`（无女巫，每钱包 1 次 10k） | SC | 0.3 d |
| 0.5.4 | `GameVault.sol` 的 `deposit()` 部分（withdraw 留 stub） | SC | 1 d |
| 0.5.5 | Sepolia 部署脚本 + `deployments/sepolia.json` | SC | 0.5 d |
| 0.5.6 | 链上 Indexer：监听 `SessionStarted`，confirm 5 块后触发 Session Svc | BE | 1.5 d |
| 0.5.7 | Session Svc：链上事件 → 创建 vAccount + 锁定 chaos seed | BE | 1 d |
| 0.5.8 | 前端：Wagmi v2 集成、Connect Wallet、SIWE 登录 | FE | 1 d |
| 0.5.9 | 前端：Faucet claim 流程（含余额展示、错误处理） | FE | 0.7 d |
| 0.5.10 | 前端：关卡选择 → approve + deposit | FE | 1 d |

#### DoD
- ✅ 3 合约部署到 Arbitrum Sepolia，Arbiscan 可查
- ✅ Faucet 60B USDR 余额正确，每钱包终生限 1 次
- ✅ MetaMask claim 成功，钱包余额 +10,000 USDR
- ✅ approve + deposit 后，后端 5 块内创建 session，玩家自动进入交易页
- ✅ 区块链重组 < 5 块时，session 不被错误创建

#### 演示脚本
1. 玩家访问 staging URL
2. 点 Connect Wallet → MetaMask 弹出 → 切到 Arbitrum Sepolia
3. 点 "Claim 10,000 USDR" → 签 tx → 等 confirm → 余额更新
4. 选 BTC-MED 关卡 → 输入 500 USDR → approve + deposit
5. 等几秒后端 indexer 处理 → 自动跳转到交易页
6. 用 v0.4 的能力交易（含强平）

---

### 🏁 v0.6 — End-to-End MVP ★（完整闭环）

> **目标**：玩家能拿回链上 USDR，看复盘报告。**MVP 完成**。

#### 用户故事
作为玩家，关卡结束（爆仓 / 时间到 / 主动平仓）后我能看复盘 → 点 Withdraw → 链上签 tx → USDR 回到钱包。

#### 任务清单

| # | 任务 | Owner | 估算 |
|---|---|---|---|
| 0.6.1 | `GameVault.sol` 的 `withdraw(receipt)` + EIP-712 验签 | SC | 1 d |
| 0.6.2 | ABI 共享文档 + Go reference EIP-712 hash 算法 | SC + BE | 0.3 d |
| 0.6.3 | Reporting Svc：从 NATS 读事件流 → 计算 finalEquity → 阈值签名 receipt | BE | 1.5 d |
| 0.6.4 | 事件包归档到 MinIO（S3 mock）+ Postgres 元数据 | BE | 0.5 d |
| 0.6.5 | 前端：关卡结束 modal + Withdraw 按钮 + tx 状态 | FE | 0.7 d |
| 0.6.6 | 前端：复盘报告页（PnL 曲线 + 关键指标） | FE | 1.5 d |
| 0.6.7 | 端到端集成测试（Anvil + 真实合约 + 完整服务） | 全员 | 1.5 d |
| 0.6.8 | 撮合确定性回归测试（连续 10 次重放比对 hash） | BE | 0.5 d |
| 0.6.9 | 50 玩家并发压测（撮合 p99） | BE | 0.5 d |
| 0.6.10 | slither 安全自检 + known-issues 记录 | SC | 0.3 d |
| 0.6.11 | Sepolia staging 部署 + Demo 视频 + README | 全员 | 1 d |

#### DoD
- ✅ 端到端流程：connect → claim → deposit → trade → settle → withdraw 跑通
- ✅ Receipt 签名验签正确，重复 withdraw 失败
- ✅ 复盘页正确渲染 PnL 曲线、最大回撤
- ✅ E2E 测试 CI 跑通
- ✅ 撮合确定性 10 次重放 byte-identical
- ✅ 50 并发玩家 p99 ≤ 50ms
- ✅ Sepolia staging 公网可访问，3 名团队外用户能玩通
- ✅ Demo 视频 5 分钟

#### 演示脚本（**MVP 验收**）
1. 团队外用户用 MetaMask 连 Sepolia
2. claim 10k USDR → deposit 500 USDR 进 BTC-MED
3. 进入 312 关卡，开 5x 杠杆多单
4. 暴跌中惊险逃顶，主动平仓 → 关卡结束
5. 看复盘：PnL 曲线、操作时间线
6. 点 Withdraw → MetaMask 签 → USDR 回到钱包
7. 全程录屏 → 5 分钟 Demo

---

## 4. v1.x 路线图详情（MVP 之后）

### 📦 v1.0 — Multi-Symbol（三难度全开）—— 5 d
**关键变化**：从单 symbol 升级到 per-symbol goroutine actor 架构。

| # | 任务 | 估算 |
|---|---|---|
| 1.0.1 | MarketActor 重构为 per-symbol，进程启动加载 3 个实例 | 1 d |
| 1.0.2 | BTC-EASY 关卡库 D-519-BTC + chaos_config L1 参数 | 1 d |
| 1.0.3 | BTC-HARD 关卡库 D-FTX/D-85 + chaos_config L3（含 ADL + 穿仓 + 资金费 ±1%/h） | 2 d |
| 1.0.4 | 前端关卡选择页升级（3 卡片 + 难度参数对比） | 0.5 d |
| 1.0.5 | E2E 测试覆盖 3 symbol | 0.5 d |

---

### 📦 v1.1 — Multi-Provider（数据源容灾）—— 4 d

| # | 任务 | 估算 |
|---|---|---|
| 1.1.1 | Bitget Adapter（继承 v0.1 的 IDataProvider） | 2 d |
| 1.1.2 | 多源 diff 监控：每天定时 job 对比 OHLC，> 1% 报警 | 1 d |
| 1.1.3 | Disaster Catalog 多版本支持（primary + fallback） | 1 d |

---

### 📦 v1.2 — Reward Pools + SBT（链上激励）—— 6 d

| # | 任务 | 估算 |
|---|---|---|
| 1.2.1 | RewardPool.sol 模板（带 weeklyCap + pauseThreshold + EIP-712 poolTier） | 1.5 d |
| 1.2.2 | 三个 RewardPool 实例部署（L1=10M / L2=50M / L3=100M）| 0.5 d |
| 1.2.3 | SurvivorBadge.sol（ERC-5192 SBT） | 0.5 d |
| 1.2.4 | Reporting Svc 加 RewardReceipt 生成 + payout 流程 | 1.5 d |
| 1.2.5 | 前端：通关后 "Claim Reward" 按钮 + SBT 展示 | 1 d |
| 1.2.6 | 链下监控：单池单日 / 单主体多次中奖 / 通关率 3σ 告警 | 1 d |

---

### 📦 v1.3 — Leaderboard + 段位（27 榜 + 6 段位）—— 5 d

| # | 任务 | 估算 |
|---|---|---|
| 1.3.1 | LeaderboardSvc：9 ZSet × 3 windows ZADD/ZREVRANGE | 1.5 d |
| 1.3.2 | Wash trade / self-match 检测算法（关卡结算时一次性扫） | 1.5 d |
| 1.3.3 | 段位积分公式 + Postgres `player_tier` 表 + UPSERT | 0.5 d |
| 1.3.4 | 排行榜 API + WS 推送（"你被超越了"）| 0.5 d |
| 1.3.5 | 前端：排行榜页 + 个人段位卡片 + 段位 SBT 铸造 | 1 d |

---

### 📦 v1.4 — Sybil Gate + DEX + 多签（合规与安全）—— 6 d

| # | 任务 | 估算 |
|---|---|---|
| 1.4.1 | Faucet 加 Gitcoin Passport 验证（off-chain attestation + EIP-712） | 1.5 d |
| 1.4.2 | 链上年龄检查（后端 indexer 出签名证明） | 1 d |
| 1.4.3 | Uniswap V3 USDR/USDC 池部署 + 5B+5M 流动性注入 + LP timelock 1 年 | 1.5 d |
| 1.4.4 | 多签（Safe 3-of-5）替换所有 EOA owner | 0.5 d |
| 1.4.5 | EIP-7702 / Session Key 集成（避免每笔下单弹钱包） | 1.5 d |

---

### 📦 v1.5 — Client Chaos FX（沉浸式恐惧）—— 4 d

| # | 任务 | 估算 |
|---|---|---|
| 1.5.1 | 后端下发 ClientFXConfig（per symbol） | 0.5 d |
| 1.5.2 | 前端 ClientChaosFx 调度器：UI 假死 / 假 RPC 延迟 / glitch / 屏幕抖动 | 2 d |
| 1.5.3 | 心跳音 + 警告音 + 图表回退跳变 | 1 d |
| 1.5.4 | BTC-HARD 入场免责弹窗（"我已知悉网络故障是设计的一部分"） | 0.5 d |

---

### 📦 v2.x — 多链 / Quant API / 移动端（Phase 2）

不在本规划详细范围。占位：
- v2.0 LayerZero OFT 跨 Arbitrum/Base/BNB/Sui
- v2.1 REST/WS Quant API + 机器人专用 chaos profile
- v2.2 移动端原生应用
- v2.3 公会 / 锦标赛模式

---

## 5. 模块依赖图

```
v0.1 ──► v0.2 ──► v0.3 ──► v0.4 ──► v0.5 ──► v0.6 ★ MVP
 ▲        ▲        ▲        ▲        ▲        ▲
 │        │        │        │        │        │
 │  ┌─────┴────────┴────────┴────────┴────────┴─────┐
 │  │       下层基础设施 (在 v0.1 时全部就位)            │
 │  │  Go monorepo │ Postgres │ TimescaleDB │ NATS │  │
 │  │  Redis (v1.3+) │ docker-compose │ CI/CD          │
 │  └────────────────────────────────────────────────┘
 │
 v1.0 (multi-symbol) ──┬──► v1.2 (reward pool + SBT)
 v1.1 (multi-provider) ┤
                       └──► v1.3 (leaderboard) ──► v1.4 (sybil/DEX/multisig)
                                                    │
                                                    └──► v1.5 (client chaos)
                                                          │
                                                          └──► v2.x
```

---

## 6. 每个版本的"砍点"（Cut Lines）

每个版本必须能砍——如果时间不够，砍掉这些**仍能演示**：

| 版本 | 必保 | 可砍（应急） |
|---|---|---|
| v0.1 | OrderBook + Replay | inspector 做粗糙输出（fmt.Println） |
| v0.2 | CLI 下市价单 + 持仓 | 限价单 / 撤单（推 v0.3） |
| v0.3 | 浏览器看订单簿 + 下单 | K 线图（用粗糙 SVG）/ 历史成交列表 |
| v0.4 | 服务端 chaos + 强平 | 前端动画（v1.5 再做）/ 资金费率（推 v0.6） |
| v0.5 | 钱包连 + claim + deposit | 多链支持（v2 才做） |
| v0.6 | withdraw + 复盘曲线 | 高级复盘指标（v1+） |

---

## 7. 验收 KPI 演进表

按版本累积：

| KPI | v0.1 | v0.2 | v0.3 | v0.4 | v0.5 | v0.6 ★ |
|---|---|---|---|---|---|---|
| 撮合确定性 byte-identical | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| OrderBook 单 goroutine ≥ 50k ord/s | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| CLI 下单到 fill < 100ms | — | ✅ | ✅ | ✅ | ✅ | ✅ |
| 浏览器订单簿刷新 ≤ 100ms | — | — | ✅ | ✅ | ✅ | ✅ |
| 强平触发延迟 ≤ 1 tick | — | — | — | ✅ | ✅ | ✅ |
| 不变量自检每 tick 通过 | — | — | — | ✅ | ✅ | ✅ |
| Sepolia 合约部署成功 | — | — | — | — | ✅ | ✅ |
| Faucet claim 成功（钱包 +10k） | — | — | — | — | ✅ | ✅ |
| Deposit → 后端 5 块内创建 session | — | — | — | — | ✅ | ✅ |
| Withdraw 链上 tx 成功 | — | — | — | — | — | ✅ |
| 端到端集成测试通过 | — | — | — | — | — | ✅ |
| 50 玩家并发 p99 ≤ 50ms | — | — | — | — | — | ✅ |
| Demo 视频 5 分钟 | — | — | — | — | — | ✅ |

---

## 8. 风险登记 (Risk Register)

| ID | 风险 | 概率 | 影响 | 缓解版本 |
|---|---|---|---|---|
| R1 | Binance D-312 历史 L2 订单簿拿不到 | 高 | 高 | **v0.1 必须验证**；v0.4 兜底 depth_synth |
| R2 | 撮合非确定性导致重放 hash 漂 | 中 | 高 | v0.1 / v0.4 强制确定性测试，禁用 time.Now() |
| R3 | 前端订单簿渲染卡顿 | 中 | 中 | v0.3 用 delta 协议 + Canvas 渲染 |
| R4 | EIP-712 typehash 跨语言不一致 | 中 | 高 | v0.6 SC + BE 共享 reference impl |
| R5 | Sepolia 网络故障 | 低 | 中 | E2E 主跑 Anvil，Sepolia 仅 staging |
| R6 | 关键人请假 | 中 | 高 | 周会同步进度，关键任务有 backup |

---

## 9. 不在 MVP 但需预留接口（避免 v1.x 重构）

写代码时**留好扩展点**：

| 事项 | 预留方式 |
|---|---|
| 多 symbol | OrderBook / MarketActor / ChaosConfig 已 per-symbol 参数化 |
| RewardPool | Receipt schema 包含 `rewardAmt` / `badges[]`（MVP 留空） |
| Leaderboard | Reporting Svc 计算 `cumNotional` / `realizedPnL` 但不写榜（v1.3 加 LeaderboardSvc 调用即可） |
| 反女巫 | Faucet 合约保留 `_verifySybil(proof)` 钩子 |
| 多签 | 部署脚本支持 owner 后期 `transferOwnership` 到 Safe |
| 客户端 chaos | 前端预留 `ClientFXConfig` 解析 + handler stub |
| 自定义关卡 | Disaster Catalog 表已经支持任意 `level_id` 注入 |

---

## 10. 持续交付节奏建议

- **每 vX.Y 完成 → tag git → 部署 staging → 团队内演示 30 分钟**
- **演示后立即 retro**：哪些假设错了？哪些技术债压不住？下版本调整范围
- **测试金字塔**：每版本必须配套测试（unit + integration + e2e），缺测试不算完成
- **观测先行**：v0.4 起加 Prometheus，每个新功能都暴露 metrics

---

## 11. 关键决策记录（已对齐 requirements.md）

- ✅ 撮合在沙盒内进行，不真实清算 → 合规风险低
- ✅ USDR 标准 ERC-20，无 transfer hook → 简单 + 可在 Uniswap 自由流通
- ✅ 后端撮合**完全确定**，所有"恶意"前置到 ChaosConfig + 客户端 FX
- ✅ Per-symbol goroutine actor → 故障域隔离 + 易扩展
- ✅ 多源数据接入抽象 IDataProvider → 后期容灾不重构
- ✅ Receipt commit-reveal 模式 → 玩家可验证公平性
- ✅ MVP 简化：单 symbol、单关卡、单数据源、无 RewardPool、无 Leaderboard、无 SBT
- ✅ Phase 1 一步步加：v1.0 → v1.5

---

> **"完美是优秀的敌人。先让 v0.1 跑起来，再说一切。"**
