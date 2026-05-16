# Perp Crisis Sandbox — 技术架构设计 (Design)

> 本文档承接 `requirements.md`。架构哲学：
> **后端纯净撮合 + 多源数据接入 + 多 symbol 隔离 + 客户端体感混沌 + 链上 USDR 经济**。

---

## 1. 总体架构 (High-Level Architecture)

```
                              ┌──────────────────────────────────────────────┐
                              │              Web / Mobile Client             │
                              │ ┌──────────────────────────────────────────┐ │
                              │ │      Client Chaos FX (UI Layer)          │ │
                              │ │   • UI freeze / "Connecting..."          │ │
                              │ │   • Fake RPC delay on order ack          │ │
                              │ │   • Screen shake / glitch / SFX          │ │
                              │ │   • Driven by per-symbol chaos config    │ │
                              │ └──────────────────────────────────────────┘ │
                              │   Next.js + WebGL Chart + WS + Wagmi/Viem    │
                              └─────────────────┬───────────────┬────────────┘
                                  下单(REST/WS) │               │ 行情/账户(WS)
                                                ▼               ▼
                              ┌──────────────────────────────────────────────┐
                              │            API Gateway (Edge / NGINX)        │
                              │     SIWE + Session Key / RateLimit / IP geo  │
                              └─────────────────┬────────────────────────────┘
                                                │
        ┌──────────┬──────────┬─────────────────┼─────────────────┬───────────┬───────────┐
        ▼          ▼          ▼                 ▼                 ▼           ▼           ▼
 ┌───────────┐┌──────────┐┌──────────┐ ┌─────────────────┐ ┌──────────┐┌──────────┐┌──────────┐┌──────────────┐
 │ Session   ││ Order    ││ Market   │ │ Chaos Config    │ │ Replay   ││ Reporting││ Faucet/  ││ Leaderboard  │
 │ Svc       ││ Svc      ││ Data WS  │ │ Svc (per symbol)│ │ Svc      ││ Svc      ││ Reward   ││ Svc          │
 │ (Go)      ││ (Go)     ││ Fanout   │ │ (Go) hot-reload │ │ (Go)     ││ (Python) ││ Indexer  ││ (Go) ZADD/WS │
 └─────┬─────┘└─────┬────┘└────▲─────┘ └────────┬────────┘ └────┬─────┘└────▲─────┘└────┬─────┘└──────▲───────┘
       │            │          │                │               │           │           │
       │      ┌─────▼──────────┴────────────────▼───────────────▼───┐       │           │
       │      │          Match Engines (one goroutine per symbol)   │       │           │
       │      │  ┌───────────────┐ ┌───────────────┐ ┌────────────┐ │       │           │
       │      │  │ BTC-EASY book │ │ BTC-MED book  │ │ BTC-HARD ..│ │───────┘           │
       │      │  │ + chaos cfg L1│ │ + chaos cfg L2│ │ + chaos cfg│ │                   │
       │      │  │ + accounts L1 │ │ + accounts L2 │ │ + accounts │ │                   │
       │      │  └───────────────┘ └───────────────┘ └────────────┘ │                   │
       │      └────────────────────┬────────────────────────────────┘                   │
       │                           │ append-only event stream                            │
       │                           ▼                                                     │
       │            ┌────────────────────────────────────┐                               │
       │            │     NATS JetStream (Event Store)   │ ←── audit / replay            │
       │            └─────────────────┬──────────────────┘                               │
       │                              ▼                                                  │
       │            ┌────────────────────────────────────┐                               │
       └───────────►│  TimescaleDB  (历史 Tick 库)       │                               │
                    │  Postgres     (账户 / 会话 / chaos config)                          │
                    │  Redis        (热缓存 / 排行榜)                                     │
                    └────────────────▲───────────────────┘                               │
                                     │                                                   │
        ┌────────────────────────────┴───────────────┐                                   │
        │       Data Ingestion (multi-provider)      │                                   │
        │  ┌──────────────┐  ┌──────────────┐        │                                   │
        │  │ Binance      │  │ Bitget       │  ...   │                                   │
        │  │ Adapter (Go) │  │ Adapter (Go) │        │                                   │
        │  └──────┬───────┘  └──────┬───────┘        │                                   │
        │         └─── normalize ───┴─► Tick schema  │                                   │
        └────────────────────────────────────────────┘                                   │
                                                                                         │
┌──────────────────────────── On-chain Layer ──────────────────────────────────────┐     │
│  USDR.sol (ERC-20, 1000亿)         USDRFaucet.sol (10k claim)                    │
│  GameVault.sol (deposit/withdraw)  SurvivorBadge.sol (SBT)                       │◄────┘
│  RewardPool L1.sol (10M  奖池)     RewardPool L2.sol (50M 奖池)                  │
│  RewardPool L3.sol (100M 奖池)     Uniswap V3 USDR/USDC                          │
│  Arbitrum / Base 主部署，Phase2 LayerZero OFT 多链   (官方提供初始流动性)         │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 模块详细设计

### 2.1 Data Ingestion (Multi-Provider Adapter Layer)

**职责**：把不同来源的历史与实时行情归一化为统一 `Tick` schema，落入 TimescaleDB。

**抽象接口**：
```go
type IDataProvider interface {
    Name() string                                              // "binance" | "bitget" | ...
    FetchKlines(symbol string, t1, t2 time.Time, interval string) ([]Kline, error)
    FetchOrderbookSnapshot(symbol string, t time.Time) (OrderBookSnapshot, error)
    FetchAggTrades(symbol string, t1, t2 time.Time) ([]AggTrade, error)
    FetchFunding(symbol string, t1, t2 time.Time) ([]FundingPoint, error)
    StreamLive(ctx context.Context, symbols []string) (<-chan Tick, error) // 可选
}
```

**Provider 适配**：
| Provider | 历史 K 线 | 历史订单簿 | 历史成交 | 资金费率 | 备注 |
|---|---|---|---|---|---|
| **Binance** | ✅ REST `/fapi/v1/klines` | ⚠️ 仅近期，深度归档需自爬 WS | ✅ aggTrades.zip | ✅ | 主要源 |
| **Bitget** | ✅ REST | ⚠️ 同上 | ✅ | ✅ | 备选源 |
| **Tardis.dev** (Phase 2 付费) | ✅ | ✅ 全历史 L2 | ✅ | ✅ | 付费但深度最全 |

**归一化 Tick schema**：
```go
type Tick struct {
    TS          int64           // ns，UTC
    Provider    string          // "binance" | "bitget"
    Symbol      string          // 内部 symbol，如 "BTC-MED"
    UnderlyingSymbol string     // "BTCUSDT"，便于追溯
    LastPrice   decimal.Decimal
    IndexPrice  decimal.Decimal
    MarkPrice   decimal.Decimal
    Bids        []PriceLevel    // L2 深度（缺失则 nil，运行时合成）
    Asks        []PriceLevel
    Trades      []PublicTrade
    FundingRate decimal.Decimal
}
```

**Disaster Catalog**（运营在后台维护）：
```sql
CREATE TABLE disaster_levels (
    level_id      TEXT PRIMARY KEY,    -- 'D-312-BTC'
    name          TEXT,
    primary_provider TEXT,             -- 'binance'
    fallback_providers TEXT[],         -- {'bitget'}
    underlying_symbol TEXT,            -- 'BTCUSDT'
    time_start    TIMESTAMPTZ,
    time_end      TIMESTAMPTZ,
    sha256        TEXT,                -- dataset 完整性
    description   TEXT
);
```

**多源对比**：每个关卡 dataset 落库时同时拉 primary + fallback，运营在后台 diff 价差曲线，选择"最戏剧化"的版本作为权威；fallback 仅作为校验与备份。

**深度合成 fallback**：当 provider 没有 L2 历史快照时，用 K 线 OHLCV + 成交流量按 power-law 合成订单簿深度（公式见 `replay/depth_synth.go`），保证撮合可继续。

---

### 2.2 Chaos & Replay Engine

**职责**：从 TimescaleDB 读出原始 Tick，叠加 per-symbol 的服务端 chaos，按 chaos clock 串行喂给对应的 Match Engine。

**双层 chaos 设计**：

| 层 | 在哪里执行 | 影响什么 | 决定性 |
|---|---|---|---|
| **Server-side Chaos** | 后端 `Chaos Config Svc` + Match Engine | 真实成交结果（插针、深度、资金费、mark 滞后） | 完全确定，由 (symbol, seed, tick_idx) 决定 |
| **Client-side Chaos FX** | 前端 React | 仅体感（UI 假死、抖动、声音、假 RPC 延迟） | 客户端独立随机，不影响后端 |

**ChaosConfig（per symbol）**：
```go
type ChaosConfig struct {
    Symbol         string         // BTC-EASY / BTC-MED / BTC-HARD

    // ── server-side knobs (撮合层真实生效) ─────────
    Seed           uint64         // 关卡开始时锁定
    WickProb       float64        // 插针概率 / tick
    WickMagnitude  float64        // 插针幅度 (e.g. 0.05)
    DepthShrink    float64        // 深度收缩比例 (1.0 = 原样, 0.05 = 砍到 5%)
    OracleLagMs    int64          // mark price 滞后毫秒
    FundingBoost   decimal.Decimal // 资金费率叠加
    EnableADL      bool

    // ── client-side knobs (仅推给前端，让前端自行表演) ─
    ClientFX       ClientFXConfig
}

type ClientFXConfig struct {
    UIFreezeProbPerMin   float64  // e.g. 0.5 = 半数概率每分钟假死一次
    UIFreezeMinDurMs     int64
    UIFreezeMaxDurMs     int64
    FakeOrderAckLagMs    int64    // 前端故意延迟显示 ack
    GlitchProbPerSec     float64  // 屏幕闪屏概率
    ChartRollbackProb    float64  // 图表跳变概率
    ScarySFX             bool     // 启用心跳音、警告音
}
```

**Chaos pipeline**（每个 tick 串行，纯函数 + 种子化随机）：
```
原始Tick ─► 插针注入 ─► 深度收缩 ─► MarkPrice 滞后 ─► 资金费叠加 ─► Tick'
                                                                    │
                          ┌─────────────────────────────────────────┘
                          ▼
                    Match Engine (per symbol)
                          │
                          └──► WS Fanout ──► Client (Tick' + ClientFXConfig)
                                                  │
                                                  └── 客户端按 FX 配置决定是否假死/抖动
```

**确定性保证**：
- 所有 server-side 随机源 `xorshift(seed, symbol_id, tick_index)`，禁止使用墙钟。
- 同一 (symbol, seed) 重放必产出**完全一致**的成交流，方便审计、回放、复现 bug。
- Client FX 是非确定的、个体化的——不同玩家在不同时刻被假死，但**所有玩家的撮合结果完全相同**，公平性由后端保证。

---

### 2.3 Match Engine Core (Per-Symbol Isolation)

**实现选择**：**Go + 每个 symbol 一个独立 goroutine**，Actor 模型。

```go
// 进程启动时为每个 symbol 启一个独立 actor
type MarketActor struct {
    Symbol     string                  // BTC-EASY / BTC-MED / BTC-HARD
    Book       *OrderBook              // 该 symbol 独占
    Accounts   *AccountStateMachine    // 该 symbol 内的玩家账户
    Chaos      *ChaosEngine            // 该 symbol 的 chaos config
    OrderQueue chan UserOrder
    TickFeed   chan Tick
    Events     chan Event              // 出口
}

func (m *MarketActor) Run(ctx context.Context) {
    for {
        select {
        case tick := <-m.TickFeed:
            tick = m.Chaos.Apply(tick)
            m.Book.InjectReplayOrders(tick)
            m.processPendingUserOrders()
            m.Book.Match()
            m.Accounts.MarkToMarket(tick.MarkPrice)
            m.Accounts.CheckLiquidations()
            m.emitEvents()
        case order := <-m.OrderQueue:
            m.Book.Submit(order)
        case <-ctx.Done():
            return
        }
    }
}
```

**为什么 per-symbol 隔离**：
- 三种难度的 chaos 参数完全不同，混在一起会让代码充满 if-else；
- 隔离后**故障域独立**：BTC-HARD 出 bug 不影响 BTC-EASY；
- 单独扩容：BTC-MED 玩家最多时可以分配更多 CPU；
- 数据隔离：每个 symbol 独立 event stream `events.btc-med.jsonl`，便于审计。

**订单簿数据结构**：
- 价格档位用**跳表 (SkipList)**：插入/删除 O(log n)，遍历最优档 O(1)；
- 每档下挂双向链表（FIFO，时间优先）；
- 单引擎实测可承载 50k–200k orders/sec。

**关键不变量 (Invariants)**：
- 对每个 symbol 独立成立：`Σ(玩家净值) + 保险基金 = 玩家累计入场 USDR`；
- 任何强平产生的对手方必须是真实存在的挂单或保险基金，不允许凭空成交。

---

### 2.4 Account State Machine

**保证金模型**（per symbol per player）：

| 公式 | 说明 |
|---|---|
| `uPnL = size × (markPrice - avgEntry)` | 多单未实现盈亏 |
| `uPnL = size × (avgEntry - markPrice)` | 空单未实现盈亏 |
| `marginRatio = (walletBalance + uPnL) / positionNotional` | 保证金率 |

当 `marginRatio ≤ MMR` 触发强平。

**全仓 vs 逐仓**：
- 全仓：账户内所有仓位共享 walletBalance；
- 逐仓：每仓独立 isolatedMargin。

**资金费率**：每 8 小时（历史原版）或加速版（每 1 小时）结算：
```
funding = positionSize × markPrice × fundingRate
```
BTC-HARD 叠加 chaos boost，最高 ±1%/h。

---

### 2.5 Crisis Liquidation Module

**清算瀑布**：
```
1. 触发 (marginRatio ≤ MMR)
       │
       ▼
2. 接管：用户仓位转入 Liquidator 账户
       │
       ▼
3. 市场出清（CLOB 市价单）
       │
       ├── 出清成功 → 剩余保证金归还用户
       ├── 部分成交 → 走兜底
       │
       ▼
4. 保险基金以 MMR 价格接盘
       │
       ├── 基金充裕 → 完成
       │
       ▼
5. ADL（仅 BTC-HARD）：按盈利百分比 × 杠杆排序，
   强制减仓盈利对手方
       │
       ▼
6. 穿仓展示：净值为负 → UI 显示，不向链上追索
```

**ADL 排序**：`adlScore = uPnL% × leverage`（最高者优先被减仓）。

---

### 2.6 Web3 Gate (USDR 经济层)

**合约清单 (Solidity, Arbitrum / Base)**：

```solidity
// 1. 普通 ERC-20，无 transfer hook
contract USDR is ERC20 {
    constructor(address ecosystemSafe) ERC20("Crisis Dollar", "USDR") {
        // 一次性 mint 1000 亿，分发到各金库；之后 mint 权限永久 renounce
        _mint(faucetVault,     60_000_000_000 ether);  // 60%
        _mint(rewardPoolL1,        10_000_000 ether);  // 0.01%   - BTC-EASY
        _mint(rewardPoolL2,        50_000_000 ether);  // 0.05%   - BTC-MED
        _mint(rewardPoolL3,       100_000_000 ether);  // 0.1%    - BTC-HARD
        _mint(teamLockup,      10_000_000_000 ether);  // 10%
        _mint(initialLP,        5_000_000_000 ether);  // 5%
        _mint(ecosystemSafe,   24_840_000_000 ether);  // 24.84% (含奖池补充储备)
    }
    // 没有 mint() / burn() 暴露给外部
}

// 2. 反女巫 + 一次性领取
contract USDRFaucet {
    mapping(address => bool) public claimed;

    function claim(bytes calldata sybilProof) external {
        require(!claimed[msg.sender], "already claimed");
        require(_verifySybil(msg.sender, sybilProof), "sybil check failed");
        claimed[msg.sender] = true;
        USDR.transfer(msg.sender, 10_000 ether);
        emit Claimed(msg.sender);
    }
}

// 3. 关卡入场金库（per symbol 独立账本）
contract GameVault {
    struct Session {
        address player;
        bytes32 symbol;            // BTC-EASY / BTC-MED / BTC-HARD
        uint256 deposited;
        bool    closed;
    }
    mapping(bytes32 => Session) public sessions;
    mapping(address => mapping(bytes32 => uint256)) public locked;

    function deposit(bytes32 symbol, uint256 amount) external returns (bytes32 sessionId) {
        require(amount >= minTicket[symbol], "too small");
        USDR.transferFrom(msg.sender, address(this), amount);
        sessionId = keccak256(abi.encode(msg.sender, symbol, block.number, nonce++));
        sessions[sessionId] = Session(msg.sender, symbol, amount, false);
        locked[msg.sender][symbol] += amount;
        emit SessionStarted(msg.sender, sessionId, symbol, amount);
    }

    function withdraw(bytes calldata receipt) external {
        (bytes32 sid, uint256 finalEquity) = _verify(receipt);
        Session storage s = sessions[sid];
        require(!s.closed && s.player == msg.sender, "bad session");
        s.closed = true;
        USDR.transfer(msg.sender, finalEquity);
        emit SessionClosed(s.player, sid, finalEquity);
    }
}

// 4. 分级奖池（三个独立部署：L1/L2/L3）
contract RewardPool {
    bytes32 public immutable poolTier;          // "L1" | "L2" | "L3"
    uint256 public immutable initialBalance;    // 10M / 50M / 100M
    uint256 public weeklyCap;                   // initialBalance * 1%
    uint256 public pauseThreshold;              // initialBalance * 5%

    mapping(uint256 => uint256) public weeklyPaidOut;        // ISO week => USDR
    mapping(address => mapping(bytes32 => bool)) public claimedLevels;  // 防重复领奖

    address public multisig;                    // 3-of-5

    function payout(bytes calldata receipt) external {
        // EIP-712 domain 包含 poolTier，避免跨池伪造
        (address player, bytes32 levelId, uint256 amount, uint256[] memory badges)
            = _verify(receipt, poolTier);

        require(!claimedLevels[player][levelId], "already claimed for this level");
        require(amount <= maxPayoutPerSession, "abuse");

        // 安全闸门
        require(_remaining() > pauseThreshold, "pool paused: refill needed");
        uint256 wk = _isoWeek(block.timestamp);
        require(weeklyPaidOut[wk] + amount <= weeklyCap, "weekly cap reached");

        claimedLevels[player][levelId] = true;
        weeklyPaidOut[wk] += amount;
        USDR.transfer(player, amount);

        for (uint i; i < badges.length; ++i) {
            SurvivorBadge.mint(player, badges[i]);
        }
        emit Payout(player, levelId, amount);
    }

    // 多签补充
    function refill(uint256 amount) external onlyMultisig {
        USDR.transferFrom(ecosystemSafe, address(this), amount);
        emit Refilled(amount, _remaining());
    }

    // 紧急暂停（多签）
    function emergencyPause() external onlyMultisig { paused = true; }

    function remaining() public view returns (uint256) { return _remaining(); }
}

// 5. SBT
contract SurvivorBadge is ERC5192 { /* non-transferable */ }
```

**资金流（端到端）**：

```
┌─ 1. 钱包获取 USDR ──────────────────────────────────────────┐
│  路径 A: USDRFaucet.claim()  → 10,000 USDR (一次性)         │
│  路径 B: Uniswap USDR/USDC   → 任意数量 (1 USDC ~ 1k USDR)  │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─ 2. 进入关卡 ───────────────────────────────────────────────┐
│  player → USDR.approve(GameVault, amount)                  │
│  player → GameVault.deposit("BTC-MED", 500)                │
│           emits SessionStarted(player, sid, "BTC-MED", 500)│
└──────────────────┬──────────────────────────────────────────┘
                   │ Faucet/Reward Indexer 监听事件
                   ▼
┌─ 3. 后端开局 ───────────────────────────────────────────────┐
│  Session Svc:                                               │
│    - 创建 vAccount(sid) 在 BTC-MED MarketActor              │
│    - 入金 500 USDR (沙盒余额)                               │
│    - 锁定 chaos seed = keccak(sid || serverCommit)          │
│  Player WS connect /ws/market/BTC-MED & /ws/account/{sid}   │
└──────────────────┬──────────────────────────────────────────┘
                   │ 关卡进行中：交易、强平、资金费 ...
                   ▼
┌─ 4. 关卡结算 ───────────────────────────────────────────────┐
│  MatchEngine emits SessionClosed(sid, finalEquity, achievements)
│  Reporting Svc:                                             │
│    - 落事件包到 S3，hash 写入 Postgres                       │
│    - 生成 Receipt {sid, finalEquity, ...}（仓位本金回收）   │
│    - 生成 RewardReceipt {sid, player, levelId, rewardAmt,   │
│             badges, poolTier="L2"} 给对应池子                │
│    - 阈值签名                                                │
│  player → GameVault.withdraw(receipt)        → 收回 finalEquity
│  player → RewardPoolL2.payout(rewardReceipt) → 收奖励 USDR + SBT
│           （poolTier 必须与 sessionId 的 symbol 匹配，避免跨池）
└─────────────────────────────────────────────────────────────┘
```

**反女巫 (Sybil Resistance)**（与 USDR 是否可转无关，仍然必要——避免 1 人薅 6000 万钱包）：
- Gitcoin Passport ≥ 15 分（off-chain attestation，链上验签）；
- 钱包链上年龄 ≥ 60 天 + ≥ 1 笔历史交易（后端 indexer 出签名证明）；
- 可选 0.001 ETH 锁，关卡完成后退还。

**Uniswap LP 设置**：
- 部署 Uniswap V3 USDR/USDC pool；
- 项目方按 1 USDC : 1,000 USDR 注入初始流动性（5B USDR + 5M USDC）；
- LP NFT **timelock 1 年**，链上可验证；
- 价格区间 [800, 1500] USDR/USDC，集中流动性减少滑点。

**多链聚合 (Phase 2)**：USDR 通过 LayerZero OFT 跨链；GameVault 在每条链独立部署，但 SessionId 全局 unique（链 ID + nonce）。

---

### 2.7 数据层

| 用途 | 选型 | 备注 |
|---|---|---|
| 历史 Tick | **TimescaleDB**（hypertable + 列压缩） | 单标的一年 100ms tick ~30GB；压缩后 ~3GB |
| 账户/会话 | Postgres | 强一致写 |
| Chaos config | Postgres `chaos_config(symbol, params jsonb)` | 热更新；变更落审计表 |
| Disaster catalog | Postgres `disaster_levels` | 关卡定义 |
| 排行榜（详见 §2.9） | Redis Sorted Set（热）+ Postgres（冷归档） | per (board, symbol, window) |
| 段位 / 积分 | Postgres `player_tier(address, score, tier, updated_at)` | 跨 symbol 累计 |
| 事件流 | NATS JetStream stream=`events.{symbol}` | per-symbol 隔离 |
| 回放包 | S3 / R2，path=`replays/{sessionId}.jsonl.gz + .sha256` | 客户端可下载 |

---

### 2.8 前端 (Client Chaos Engine)

> 关键变化：**所有"拔网线 / 抖动 / 卡顿"等体感效果由前端负责**，后端只下发 `ClientFXConfig`。

```ts
// 关卡开始时收到一次 chaos config
type ClientFXConfig = {
  uiFreezeProbPerMin: number;
  uiFreezeMinDurMs: number;
  uiFreezeMaxDurMs: number;
  fakeOrderAckLagMs: number;
  glitchProbPerSec: number;
  chartRollbackProb: number;
  scarySFX: boolean;
};

// 客户端 chaos 调度器
class ClientChaosFx {
  constructor(private cfg: ClientFXConfig) {}

  start() {
    // 每分钟概率性调度一次 UI freeze
    setInterval(() => {
      if (Math.random() < this.cfg.uiFreezeProbPerMin) this.freezeUI();
    }, 60_000);

    // 每秒概率性 glitch
    setInterval(() => {
      if (Math.random() < this.cfg.glitchProbPerSec) this.flashGlitch();
    }, 1_000);
  }

  freezeUI() {
    const dur = randBetween(this.cfg.uiFreezeMinDurMs, this.cfg.uiFreezeMaxDurMs);
    overlayStore.show("Connecting...");
    blockOrderInput(true);
    setTimeout(() => {
      overlayStore.hide();
      blockOrderInput(false);
    }, dur);
  }

  // 下单时延迟显示 ack（不影响后端实际执行时间）
  decorateOrderAck(promise: Promise<OrderAck>): Promise<OrderAck> {
    return promise.then(ack =>
      new Promise(resolve =>
        setTimeout(() => resolve(ack), this.cfg.fakeOrderAckLagMs)
      )
    );
  }
}
```

**为什么必须放在前端**：
1. **后端必须保持纯净与确定性**：撮合内核里有任何"sleep"都会污染重放、破坏审计。
2. **不影响公平性**：所有玩家的撮合结果由后端确定，前端的"卡顿"只是体感装饰，玩家的实际成交价、成交时间、强平时间在后端早已决定，前端假死 5 秒不会让你"少亏"或"多赚"。
3. **可玩性设计**：前端可以为同一 chaos config 演化出无数种"卡顿 UI 方案"（黑屏、回滚、错位），不受后端约束。

**心理学增强**：
- 心跳音 (`scarySFX = true`)、警告闪屏；
- BTC-HARD 入场前必须勾选"我已知悉网络故障是设计的一部分"。

---

### 2.9 Leaderboard Service (排行榜服务)

**职责**：在关卡结算时把成绩写入 9 个独立榜单 + 段位积分；提供高吞吐查询。

#### 2.9.1 写入路径

```
MatchEngine emits SessionClosed(sid)
            │
            ▼
   Reporting Svc 计算 stats:
     • clearTimeMs       (若通关)
     • cumNotional       (扣除 self-match + wash 折算后)
     • realizedPnL       (= withdrawAmount - depositAmount, 不含 ADL/穿仓)
     • achievements      (用于 SBT)
     • tierScore         (按 FR-7A.4 公式)
            │
            ▼
   LeaderboardSvc.Submit(sid, stats):
     ┌─ 9 个 Redis ZADD (board × symbol)
     │    ZADD lb:speedrun:BTC-MED:w:2026W20  <clearTime>  <addr>
     │    ZADD lb:volume:BTC-MED:w:2026W20    <cumNotional> <addr>
     │    ZADD lb:pnl:BTC-MED:w:2026W20       <realizedPnL> <addr>
     │    （同时 daily / alltime 各一份，共 9 × 3 = 27 个 ZADD）
     │
     ├─ Postgres UPSERT player_tier
     │    UPDATE score = score + tierScore, recompute tier band
     │
     └─ Pub WS event leaderboard.{symbol}.{board}.{window}
            │
            ▼
   订阅了该 topic 的客户端收到排名变化
```

**关键点**：
- 每个 (board, symbol, window) 都是一个 Redis Sorted Set，**`speedrun` 用升序排序**（最快第一），其他用降序——通过传 `-clearTimeMs` 作为 score 实现。
- ZADD 用 `GT` 标志（`speedrun` 用 `LT`）保证只在更优时更新，与"取最优"语义匹配。
- 段位写入是 Postgres 强一致，避免 Redis 异步丢数据导致段位"倒退"。
- 整个写入是**幂等的**——同 sid 重复触发不会重复加分（用 `processedSessions` 表去重）。

#### 2.9.2 读取路径

| 接口 | 实现 |
|---|---|
| Top N | `ZREVRANGE` (volume/pnl) / `ZRANGE` (speedrun) + `MGET` 拉用户元数据 |
| 个人排名 | `ZREVRANK` / `ZRANK` 得 0-based 名次 |
| 段位 | Postgres 单表查询，5 ms 内返回 |
| WS 实时推送 | LeaderboardSvc 在 ZADD 后比较 prev/cur rank，rank 变化时 publish |

**缓存策略**：Top 100 公榜在内存缓存 5 秒（不会变那么快），减小 Redis QPS。

#### 2.9.3 Wash Trade / Self-Match 检测

实现在 Reporting Svc 里，关卡结算时一次性扫一遍事件流：

```go
func computeFairVolume(events []Fill) decimal.Decimal {
    var raw, wash decimal.Decimal
    var positionTimeline []PositionPoint

    for _, f := range events {
        // 1. self-match: maker 与 taker 是同一玩家 → 不计
        if f.MakerAddr == f.TakerAddr {
            continue
        }
        raw = raw.Add(f.Notional)
        positionTimeline = append(positionTimeline, ...)
    }

    // 2. wash detect：60s 滑窗内净持仓时长 / 总时长 < 5% → 该窗口量打 0.5x
    for window := range slidingWindows(positionTimeline, 60s) {
        if window.NetPositionDuration / window.TotalDuration < 0.05 {
            wash = wash.Add(window.Volume * 0.5)
        }
    }

    return raw.Sub(wash)
}
```

> 这个算法**只在结算时跑一次**，不影响撮合性能；但需要保留每个 session 的事件流，已经是必备能力（用于回放）。

#### 2.9.4 段位重算与回滚

- **正常路径**：每次 SessionClosed 增量加分；
- **异常发现**（比如某玩家被检测出新型作弊）：运营调用 `tier_recompute(address)`，重读该地址所有历史 session 重算积分；这是一次性管理操作，由多签授权。
- **奖池补发**：如果检测出某周榜被作弊污染，多签可以 `rewardPool.recall(week)` 暂停下周发放，运营手动重算 + 重发 receipt。

#### 2.9.5 数据规模估算

- 假设日活 1 万玩家，每人每天平均跑 2 个关卡 → 2 万 sessions/day；
- 每 session 写 27 个 ZADD + 1 个段位 UPSERT = 28 次写 → 56 万次写/天；
- Redis 单节点轻松扛；Postgres 段位表用 `(address)` PRIMARY KEY，UPSERT 也很快。
- 每周日榜重置：把上周数据归档到 Postgres `leaderboard_archive(board, symbol, window, address, score)`，便于历史查询；Redis 只保留近 30 天。

---

## 3. 关键时序 (Sequences)

### 3.1 玩家首次入场

```
Player    Wallet    Uniswap    Faucet    USDR    GameVault    Match Engine
  │ option A: Faucet claim                                              │
  ├──► claim(sybilProof) ──► Faucet ─► verify ─► USDR.transfer 10k ─►   │
  │ option B: 在 Uniswap 用 1 USDC 买 ~1000 USDR                         │
  ├──► swap(USDC → USDR) ───► Uniswap ─► USDR ───────────────────────►   │
  │
  │ approve(GameVault, 500 USDR)
  ├────────────────────────────────────►│ USDR
  │ deposit("BTC-MED", 500)             │
  ├────────────────────────────────────►│ GameVault locks USDR
  │                                     │ emit SessionStarted(sid)
  │                                     ├────────────────────► Backend
  │                                     │                       │
  │                                     │      create vAccount  │
  │                                     │      seed = keccak(sid│ || serverCommit)
  │                                     │      load ChaosConfig │ from Postgres
  │ WS connect /ws/market/BTC-MED                                │
  │ WS connect /ws/account/{sid}                                 │
  ├──────────────────────────────────────────────────────────────►
  │ ◄── snapshot + ChaosConfig{server params + ClientFXConfig} ──┤
  │                                                              │
  │  关卡开始倒计时...                                             │
```

### 3.2 客户端 Chaos：UI 假死期间下单

```
关卡进行中
   │
   ├── Client ChaosFx 触发 freezeUI(7000ms) ─► UI 显示 "Connecting..."
   │   ├── 玩家点击下单按钮 → 前端 input 被 block，给"卡死"反馈
   │   │
   │   └── 但实际：玩家可能恐慌切快捷键 / 切手机 ──┐
   │                                              │
   │   后端：完全无感知，撮合按 chaos clock 正常推进
   │                                              │
   ├── 7s 后 freezeUI 解除                         │
   │   玩家再次点单 ──► Order Svc ──► Match ──► fill
   │   ──► fakeOrderAckLagMs = 3000，前端再延迟 3s 显示 ack（伪 RPC 拥堵）
   │
   ▼
玩家此时可能已经爆仓（mark price 在卡顿 7s 内已穿强平线）
   │
   ▼
WS 推 'liquidated' 事件 ──► 前端弹窗"你被强平了"+ 心跳音 + 屏幕抖动
```

### 3.3 强平瀑布 (BTC-HARD)

```
MarkPrice 暴跌 → markToMarket → marginRatio < MMR
                     │
                     ▼
              接管 → 市价出清
                     │
              ├ 出清失败（深度被 chaos 砍到 5%）
                     │
                     ▼
              保险基金接盘
                     │
              ├ 基金不足
                     │
                     ▼
              ADL：按 (uPnL% × leverage) 排序，
              强制减仓盈利对手方
                     │
              净值 < 0 → 推 'liquidated_negative'
                     │
              前端弹"穿仓 -1,234 USDR"动画
              (链上不追索，仅展示)
```

### 3.4 关卡结算（含分级奖池 + 排行榜）

```
MatchEngine ──► Reporting Svc
   sessionId, symbol, finalEquity, achievements, eventsHash
                     │
                     ├──► LeaderboardSvc.Submit(stats)
                     │      • 9 个 Redis ZADD (board × symbol × window)
                     │      • Postgres UPSERT player_tier (累计积分 / 段位)
                     │      • WS publish leaderboard.{symbol}.{board}.{window}
                     │
                     │  根据 symbol 选择对应 RewardPool (L1/L2/L3)
                     ▼
            生成 2 张 Receipt：
              - WithdrawReceipt: sign(sid, finalEquity)
              - RewardReceipt:   sign(sid, levelId, rewardAmt, badges, poolTier)
              （poolTier 嵌入 EIP-712 domain，避免跨池伪造）
                     │
       ┌─────────────┴────────────┐
       ▼                          ▼
Player.withdraw(receipt)   Player.payout(rewardReceipt)
       │                          │
GameVault.transfer(USDR)   RewardPoolL{1|2|3}.transfer(USDR)
                                  │
                                  ├ 检查：余量 > pauseThreshold(5%)
                                  ├ 检查：weeklyPaidOut + amount ≤ weeklyCap(1%)
                                  ├ 检查：claimedLevels[player][levelId] == false
                                  ▼
                           SurvivorBadge.mint(badges[])
```

---

## 4. 难度参数表 (Chaos Matrix, 按 symbol 持久化)

| 参数 | BTC-EASY (L1) | BTC-MED (L2) | BTC-HARD (L3) |
|---|---|---|---|
| **后端层 (撮合真实生效)** | | | |
| 历史关卡池 | D-519-BTC | D-312-BTC, D-LUNA | D-FTX, D-85, 合成虚构剧本 |
| 最大跌幅 / 关卡 | 30% | 60% | 99.9% |
| 插针概率 / min | 0 | 0.05 | 0.30 |
| 插针幅度 | — | 5% | 15% |
| 深度收缩 | 1.0x | 0.2x | 0.05x |
| MarkPrice 滞后 | 0 | 3s | 0–8s 随机 |
| 资金费率 cap | ±0.05%/h | ±0.3%/h | ±1%/h |
| 杠杆上限 | 10x | 25x | 50x |
| ADL | 关闭 | 关闭 | 启用 |
| 止损可靠性 | ✅ 100% 触发 | ❌ 滑点可能穿过 | ❌ |
| 最小门票 | 100 USDR | 500 USDR | 1,000 USDR |
| 关卡时长 | 30 min | 60 min | 60 min |
| **前端层 (仅体感)** | | | |
| UI 假死概率 / min | 0 | 0 | 0.5 (~每分钟一半概率) |
| UI 假死时长 | — | — | 5–10 s |
| 假 RPC 延迟 | 0 | 0 | 1–5 s |
| 屏幕 glitch | 关 | 偶发 | 频繁 |
| 心跳音效 | 关 | 关 | 开 |
| 图表回退 | 关 | 关 | 偶发 |

---

## 5. 公平性 / 防作弊

- **服务端权威**：客户端只是 view，所有撮合状态由后端持有。
- **撮合纯净**：后端撮合内核**禁止任何随机墙钟、禁止 sleep、禁止 UI 干预**——所有"恶意"全部前置到 ChaosConfig 注入与 server-side chaos pipeline，且基于 (symbol, seed, tick_idx) 完全确定。
- **Chaos seed commit-reveal**：玩家 deposit 后立刻冻结 seed：
  ```
  seed = keccak256(sessionId || serverSecretCommit)
  ```
  局后公开 `serverSecret`，任何人可独立运行 Match Engine 验证结果（commit-reveal 模式）。
- **回放可验证**：每个 session 的事件包 + 哈希落 S3 + Postgres，玩家可下载并独立重放。
- **客户端 chaos 不影响公平**：前端假死、抖动、声音都是体感装饰，**不能改变后端撮合结果**——这是设计的硬约束。
- **签名 session key**：使用 EIP-7702 / Account Abstraction 的会话密钥，作用域 = 单 sessionId + 单 symbol，避免每笔下单弹钱包。

---

## 6. 可观测性 / 运维

- **Metrics (Prometheus)**：每个 symbol 独立标签 `symbol="BTC-MED"`：撮合延迟 p50/p95/p99、订单吞吐、强平率、ADL 次数、保险基金余额。
- **Tracing (OpenTelemetry)**：从 Order Svc 到 MarketActor 全链路。
- **审计**：每个 sessionId 的事件包 (JSONL + sha256) 永久保留；ChaosConfig 变更落 `chaos_config_audit` 表。
- **告警**：单 symbol 撮合延迟 p99 > 50ms / 玩家爆仓率 > 95% 触发告警。
- **数据健康**：每天对比 Binance vs Bitget 的同时段 OHLC，价差 > 1% 报警（可能数据源出问题）。

---

## 7. 技术选型一览

| 层 | 选型 | 备注 |
|---|---|---|
| 前端 | Next.js, React, Zustand, Tailwind, Lightweight Charts | + 自研 ClientChaosFx |
| API | Go (chi / fiber) | 与撮合同语言便于共享类型 |
| 撮合 | Go，per-symbol goroutine actor | 内存 CLOB |
| 数据接入 | Go provider adapters (binance / bitget) | 抽象 IDataProvider |
| 消息 | NATS JetStream | per-symbol stream |
| 历史库 | TimescaleDB | 列压缩省盘 |
| 缓存/榜单 | Redis Cluster | Sorted Set |
| 链 | Arbitrum / Base (Phase 1) | 低 Gas + EVM 生态 |
| 合约 | Solidity, Foundry | USDR / Faucet / GameVault / RewardPool / SBT |
| AA | EIP-7702 / Safe | session key |
| DEX | Uniswap V3 | USDR/USDC, 集中流动性 + LP timelock |

---

## 8. 代币经济 (USDR Tokenomics)

| 维度 | 参数 |
|---|---|
| 名称 / 符号 | Crisis Dollar / **USDR** |
| 标准 | **标准 ERC-20**（无 transfer hook，无 soulbound） |
| 总量硬顶 | **100,000,000,000 USDR (1000 亿)**，一次性 mint |
| 链 | Phase 1：Arbitrum / Base；Phase 2：LayerZero OFT 多链 |
| Faucet 单次 | 10,000 USDR / 钱包 / 终生 |
| Reward Pools (分级，独立合约) | **L1: 10M / L2: 50M / L3: 100M（共 1.6 亿）** |
| 计价单位 | 沙盒内 1 USDR ≡ 1 USD 历史价（仅显示口径） |
| Uniswap 初始价 | ~1 USDC = 1,000 USDR（10,000 USDR ≈ $10） |

**分配（一次性 mint 到对应金库）**：

| 用途 | 数量 | 比例 | 释放方式 |
|---|---:|---:|---|
| Claim Faucet | 60,000,000,000 | 60% | 6 百万钱包，每人 10,000 上限 |
| **Reward Pool L1** (BTC-EASY) | **10,000,000** | **0.01%** | 单局 500 USDR，单周 cap 1% |
| **Reward Pool L2** (BTC-MED) | **50,000,000** | **0.05%** | 单局 5,000 USDR，单周 cap 1% |
| **Reward Pool L3** (BTC-HARD) | **100,000,000** | **0.1%** | 单局 50,000 USDR，单周 cap 1% |
| 团队 / 开发 | 10,000,000,000 | 10% | 4 年线性，1 年 cliff |
| 初始 DEX 流动性 | 5,000,000,000 | 5% | TGE 注入，LP 锁 1 年 |
| 生态预留 / 合作（含奖池补充储备） | 24,840,000,000 | 24.84% | 多签锁定，按提案释放 |

**奖池监控与安全（落地"监控通关率"）**：
- **链上自动闸门**：每池 `weeklyCap = 池子 × 1%` + `pauseThreshold = 池子 × 5%`，超限/低余量自动拒付；
- **链下监控**：单池单日发放 > 0.5% / 单主体多次中奖 / 通关率 3σ 突变三类告警；
- **多签补充**：3-of-5 多签从生态储备 `refill()` 注入，事件链上公开。

**反通胀 / 防 dump**：
- USDR 一次性 mint，部署后 mint 权限 renounce，**永远不可增发**；
- 团队仓位 4 年线性 + 1 年 cliff；
- DEX LP 锁 1 年；
- 单局奖励上限 50,000 USDR (~$50)；
- 三池独立 + 单周 1% 上限：最坏情况下单周市场冲击 ≤ 160 万 USDR (~$1.6k)，对 50 亿初始 LP 几乎无价格影响。

**反女巫成本估算**：
- Gitcoin Passport ≥ 15：单号成本 $5–20；
- 配合 0.001 ETH 锁，单号沉没 ≥ $5；
- 即使有人造 1 万号薅 1 亿 USDR (~$100k)，需要 $50k+ 成本，且如果集中砸盘会被链上检测出（多签可临时暂停 Reward Pool 发放）。

---

## 9. 数据源与关卡数据流

```
┌──────────────────┐  ┌──────────────────┐
│ Binance REST API │  │ Bitget REST API  │   ... 后续 Bybit/OKX
└────────┬─────────┘  └────────┬─────────┘
         │ klines / aggTrades  │
         │ funding / orderbook │
         ▼                     ▼
┌──────────────────────────────────────────┐
│   Provider Adapters (Go)                 │
│   - schema 归一化                         │
│   - 时区/精度对齐                          │
│   - 错误重试 + rate limit                  │
└────────────────┬─────────────────────────┘
                 │  Tick (normalized)
                 ▼
┌──────────────────────────────────────────┐
│   Data ETL (batch + stream)              │
│   - 落 TimescaleDB                        │
│   - 计算 sha256 校验                      │
│   - 多源 diff 写入 audit 表                │
└────────────────┬─────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────┐
│   Disaster Catalog (Postgres)            │
│   level_id, primary_provider, fallback,  │
│   time_range, sha256                     │
└────────────────┬─────────────────────────┘
                 │ Replay Svc 按关卡 id 拉取
                 ▼
            Match Engine (per symbol)
```

**数据校验**：
- 关卡启动前 Replay Svc 重算 dataset sha256，与 catalog 中存档比对；
- 每天定时 job 对比 primary vs fallback 同时段 OHLC，价差 > 1% 报警；
- 多源数据可合成"加权 mark price"作为防单源被攻击的额外保护。

---

## 10. 开放问题 (Open Questions)

> Uniswap 流动性深度问题已决策：**上线后根据实际情况处理**，初版 5B USDR + 5M USDC LP 起步，监控价格曲线 + 多签储备应急。

1. **多人共 book 的撮合精度**：同一 symbol 内 1k 玩家共享 book，user limit 单与历史 replay 单同池——已用单 goroutine + (tick_ts, order_seq) 全序解决，但需要在压力测试中验证 p99 延迟稳定。
2. **客户端 chaos 是否可被禁用**：技术上玩家可以改前端代码绕过 UI 假死，把 BTC-HARD 玩成 BTC-MED——但**对实际撮合结果不产生任何影响**（后端撮合是确定的），只是降低了"难度体感"。可接受的妥协：作弊者失去乐趣但公平性不受损。
3. **数据源新成员接入门槛**：需要满足什么条件才允许加入新 provider？建议：(a) 至少 3 个标的覆盖 D-312/D-519/D-FTX；(b) 时间戳精度 ≥ 1s；(c) 公开免费 API。
4. **历史关卡的"剧本作者"**：BTC-HARD 的"合成虚构剧本"由谁定？建议引入"灾难创作者"角色，社区可以投稿剧本，由运营审核纳入关卡池——这是一个潜在的社区增长杠杆。
5. **奖池调参周期**：初版 L1=500 / L2=5,000 / L3=50,000 是基于 1,000 USDR/USDC 初始价做的估算。Uniswap 二级价格波动后，是否需要每月由多签调整 `payoutPerLevel`？建议设计 `setPayoutAmount()` 函数 + 7 天 timelock 防止突袭式提款。
