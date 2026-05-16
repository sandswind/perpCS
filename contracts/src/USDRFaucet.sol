// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title USDRFaucet — one-shot 10k-USDR drip per wallet
/// @notice Each wallet may call `claim()` exactly once and receive 10,000 USDR.
///         No sybil resistance is enforced on-chain; this is a testnet-only
///         convenience for player onboarding. Off-chain analytics may be used
///         for additional anti-abuse if needed.
/// @dev    Funded by transferring USDR from the Faucet vault to this contract
///         after deploy. The contract holds the tokens and pays them out.
contract USDRFaucet is ReentrancyGuard {
    using SafeERC20 for IERC20;

    /// @notice The USDR token this faucet hands out.
    IERC20 public immutable USDR;

    /// @notice Drip size per claim, in raw token units (18 decimals).
    uint256 public constant CLAIM_AMOUNT = 10_000 * 1e18;

    /// @notice Tracks lifetime claim per wallet.
    mapping(address account => bool) public hasClaimed;

    /// @notice Total USDR ever paid out by this contract.
    uint256 public totalClaimed;

    event Claimed(address indexed account, uint256 amount, uint256 totalClaimed);

    error AlreadyClaimed();
    error InsufficientFaucetBalance(uint256 have, uint256 need);
    error ZeroAddressToken();

    constructor(IERC20 usdr_) {
        if (address(usdr_) == address(0)) revert ZeroAddressToken();
        USDR = usdr_;
    }

    /// @notice Claim 10,000 USDR. Reverts on second call from the same address.
    /// @dev    CEI-pattern: state mutation before external transfer.
    ///         `nonReentrant` is belt-and-suspenders since we use SafeERC20.
    function claim() external nonReentrant {
        if (hasClaimed[msg.sender]) revert AlreadyClaimed();

        uint256 bal = USDR.balanceOf(address(this));
        if (bal < CLAIM_AMOUNT) revert InsufficientFaucetBalance(bal, CLAIM_AMOUNT);

        // Effects
        hasClaimed[msg.sender] = true;
        totalClaimed += CLAIM_AMOUNT;

        // Interaction
        USDR.safeTransfer(msg.sender, CLAIM_AMOUNT);

        emit Claimed(msg.sender, CLAIM_AMOUNT, totalClaimed);
    }

    /// @notice Read-only convenience for the frontend.
    function remainingBalance() external view returns (uint256) {
        return USDR.balanceOf(address(this));
    }
}
