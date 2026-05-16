// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";

/// @title USDR — Perp Crisis Sandbox testnet stablecoin
/// @notice Fixed-supply ERC-20 minted once at deploy time across five vaults.
///         There is no `mint` or `burn` function; total supply is immutable.
/// @dev    Decimals = 18 (default). 100B = 100_000_000_000 * 1e18 wei.
///
/// Vault allocation (rationale documented in design.md §v0.5):
///   1. Faucet     — funds USDRFaucet (player onboarding, ~6M wallets x 10k each)
///   2. Insurance  — backstop for liquidations and protocol-level losses
///   3. Treasury   — long-term protocol treasury / grants
///   4. Team       — core contributors (timelocked off-chain in v0.5)
///   5. Liquidity  — DEX seed liquidity for future on-chain price discovery
///
/// All five recipients are passed as constructor args so the deployer (script)
/// is the single source of truth for who holds what. The contract enforces:
///   - five non-zero recipients
///   - amounts sum to exactly 100,000,000,000 USDR (100B)
contract USDR is ERC20 {
    /// @notice Total supply minted at deployment, in whole USDR (without decimals).
    uint256 public constant TOTAL_SUPPLY_USDR = 100_000_000_000;

    /// @notice Number of vault buckets the supply is split across.
    uint256 public constant VAULT_COUNT = 5;

    /// @notice The five vault recipients in deploy order, persisted for off-chain audit.
    address[VAULT_COUNT] public vaults;

    /// @notice Per-vault allocations in raw token units (wei, 18 decimals), persisted for audit.
    uint256[VAULT_COUNT] public vaultAmounts;

    /// @dev Event for off-chain indexers / etherscan readers.
    event VaultsInitialized(address[VAULT_COUNT] vaults, uint256[VAULT_COUNT] amounts);

    error ZeroVaultAddress(uint256 index);
    error AllocationMismatch(uint256 sum, uint256 expected);

    /// @param vaults_ The five recipient addresses (Faucet, Insurance, Treasury, Team, Liquidity).
    /// @param amounts_ The five allocations, in **whole USDR** (no decimals).
    ///                 The constructor multiplies by 1e18.
    constructor(address[VAULT_COUNT] memory vaults_, uint256[VAULT_COUNT] memory amounts_)
        ERC20("USD Reserve (PerpCS)", "USDR")
    {
        uint256 sum;
        for (uint256 i = 0; i < VAULT_COUNT; i++) {
            if (vaults_[i] == address(0)) revert ZeroVaultAddress(i);
            sum += amounts_[i];
        }
        if (sum != TOTAL_SUPPLY_USDR) revert AllocationMismatch(sum, TOTAL_SUPPLY_USDR);

        // Persist for audit + emit event before any mints.
        for (uint256 i = 0; i < VAULT_COUNT; i++) {
            vaults[i] = vaults_[i];
            vaultAmounts[i] = amounts_[i] * 1e18;
        }
        emit VaultsInitialized(vaults_, vaultAmounts);

        // Mint after persistence to keep the audit log monotonic.
        for (uint256 i = 0; i < VAULT_COUNT; i++) {
            _mint(vaults_[i], vaultAmounts[i]);
        }
    }
}
