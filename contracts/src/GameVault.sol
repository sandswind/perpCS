// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title GameVault — escrows USDR for a perp-sandbox session
/// @notice v0.5 implements `deposit()` only. `withdraw()` is a stub that
///         reverts; settlement and exits land in v0.6.
/// @dev    A SessionStarted event is the single source of truth for the
///         off-chain Indexer / Session Svc. The sessionId is deterministic:
///             keccak256(player, levelId, depositNonce[player])
///         which makes it cheap to recompute client-side and unique even if
///         a player deposits twice into the same level.
contract GameVault is ReentrancyGuard {
    using SafeERC20 for IERC20;

    /// @notice The USDR token escrowed by this vault.
    IERC20 public immutable USDR;

    /// @notice Minimum deposit, in raw token units (18 decimals). 100 USDR.
    uint256 public constant MIN_DEPOSIT = 100 * 1e18;

    /// @notice Maximum deposit, in raw token units (18 decimals). 50,000 USDR.
    /// @dev    Caps single-session exposure; multiple sessions per player are
    ///         allowed but each is capped.
    uint256 public constant MAX_DEPOSIT = 50_000 * 1e18;

    /// @notice Per-player monotonic counter, used to derive sessionId.
    mapping(address player => uint256) public depositNonce;

    /// @notice Per-session escrow balance. Only the Session Svc (v0.6) reads
    ///         this; left public for transparency.
    mapping(bytes32 sessionId => uint256) public sessionBalance;

    /// @notice Maps sessionId back to the level it was opened against.
    mapping(bytes32 sessionId => bytes32) public sessionLevel;

    /// @notice Maps sessionId back to its owner. Used by withdraw (v0.6).
    mapping(bytes32 sessionId => address) public sessionOwner;

    event SessionStarted(
        address indexed player,
        bytes32 indexed levelId,
        bytes32 indexed sessionId,
        uint256 amount,
        uint256 nonce,
        uint256 blockNumber
    );

    error AmountBelowMin(uint256 amount, uint256 min);
    error AmountAboveMax(uint256 amount, uint256 max);
    error ZeroAddressToken();
    error WithdrawNotImplemented();
    error ZeroLevelId();

    constructor(IERC20 usdr_) {
        if (address(usdr_) == address(0)) revert ZeroAddressToken();
        USDR = usdr_;
    }

    /// @notice Deposit `amount` USDR to start a new session for `levelId`.
    ///         Caller must have approved this contract for at least `amount`.
    /// @return sessionId The deterministic id of the freshly opened session.
    function deposit(bytes32 levelId, uint256 amount)
        external
        nonReentrant
        returns (bytes32 sessionId)
    {
        if (levelId == bytes32(0)) revert ZeroLevelId();
        if (amount < MIN_DEPOSIT) revert AmountBelowMin(amount, MIN_DEPOSIT);
        if (amount > MAX_DEPOSIT) revert AmountAboveMax(amount, MAX_DEPOSIT);

        // Bump nonce first so sessionId is unique even on retried deposits.
        uint256 nonce = ++depositNonce[msg.sender];
        sessionId = keccak256(abi.encode(msg.sender, levelId, nonce));

        // Effects
        sessionBalance[sessionId] = amount;
        sessionLevel[sessionId] = levelId;
        sessionOwner[sessionId] = msg.sender;

        // Interaction (pulls from caller — must have prior approve())
        USDR.safeTransferFrom(msg.sender, address(this), amount);

        emit SessionStarted(msg.sender, levelId, sessionId, amount, nonce, block.number);
    }

    /// @notice v0.5 stub. Will be implemented in v0.6 with off-chain settlement
    ///         attestation, signature verification, and PnL payouts.
    function withdraw(bytes32, /* sessionId */ bytes calldata /* signature */ ) external pure {
        revert WithdrawNotImplemented();
    }

    /// @notice Convenience getter for the FE / Indexer.
    function getSession(bytes32 sessionId)
        external
        view
        returns (address player, bytes32 levelId, uint256 amount)
    {
        return (sessionOwner[sessionId], sessionLevel[sessionId], sessionBalance[sessionId]);
    }
}
