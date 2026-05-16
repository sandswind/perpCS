// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Script, console2} from "forge-std/Script.sol";
import {USDR} from "../src/USDR.sol";
import {USDRFaucet} from "../src/USDRFaucet.sol";
import {GameVault} from "../src/GameVault.sol";

/// @title Deploy — one-shot deploy script for v0.5 On-chain Entry
/// @notice Deploys USDR, USDRFaucet, GameVault and funds the faucet.
///
/// Usage:
///   source .env
///   forge script script/Deploy.s.sol:Deploy \
///     --rpc-url $ARBITRUM_SEPOLIA_RPC_URL \
///     --private-key $PRIVATE_KEY \
///     --broadcast --verify
///
/// Env vars (see .env.example):
///   PRIVATE_KEY                 deployer key
///   USDR_VAULT_FAUCET           60B vault — defaults to deployer
///   USDR_VAULT_INSURANCE        10B vault
///   USDR_VAULT_TREASURY         10B vault
///   USDR_VAULT_TEAM             10B vault
///   USDR_VAULT_LIQUIDITY        10B vault
///   FAUCET_FUNDING_USDR         whole USDR to send to USDRFaucet (default 60B)
contract Deploy is Script {
    // Allocations in whole USDR (no decimals).
    uint256 constant ALLOC_FAUCET = 60_000_000_000;
    uint256 constant ALLOC_INSURANCE = 10_000_000_000;
    uint256 constant ALLOC_TREASURY = 10_000_000_000;
    uint256 constant ALLOC_TEAM = 10_000_000_000;
    uint256 constant ALLOC_LIQUIDITY = 10_000_000_000;

    function run()
        external
        returns (USDR usdr, USDRFaucet faucet, GameVault vault, address[5] memory vaults)
    {
        address deployer = msg.sender;
        // For each vault env var, fall back to the deployer so a single key
        // can stand up the whole stack on testnet.
        vaults[0] = vm.envOr("USDR_VAULT_FAUCET", deployer);
        vaults[1] = vm.envOr("USDR_VAULT_INSURANCE", deployer);
        vaults[2] = vm.envOr("USDR_VAULT_TREASURY", deployer);
        vaults[3] = vm.envOr("USDR_VAULT_TEAM", deployer);
        vaults[4] = vm.envOr("USDR_VAULT_LIQUIDITY", deployer);

        uint256[5] memory amts =
            [ALLOC_FAUCET, ALLOC_INSURANCE, ALLOC_TREASURY, ALLOC_TEAM, ALLOC_LIQUIDITY];

        uint256 faucetFunding = vm.envOr("FAUCET_FUNDING_USDR", ALLOC_FAUCET);
        require(faucetFunding <= ALLOC_FAUCET, "Deploy: faucet funding exceeds vault");

        vm.startBroadcast();

        usdr = new USDR(vaults, amts);
        console2.log("USDR deployed at", address(usdr));

        faucet = new USDRFaucet(usdr);
        console2.log("USDRFaucet deployed at", address(faucet));

        vault = new GameVault(usdr);
        console2.log("GameVault deployed at", address(vault));

        // Fund the faucet only when the deployer is also the faucet vault.
        // Otherwise the faucet vault must call USDR.transfer manually post-deploy.
        if (vaults[0] == deployer) {
            usdr.transfer(address(faucet), faucetFunding * 1e18);
            console2.log("Faucet funded with (whole USDR)", faucetFunding);
        } else {
            console2.log(
                "Faucet vault != deployer; transfer USDR to faucet manually:", address(faucet)
            );
        }

        vm.stopBroadcast();

        _writeDeployments(usdr, faucet, vault, vaults);
    }

    function _writeDeployments(
        USDR usdr,
        USDRFaucet faucet,
        GameVault vault,
        address[5] memory vaults
    ) internal {
        // Build a JSON blob that the FE / Indexer can read.
        string memory key = "deployment";
        vm.serializeUint(key, "chainId", block.chainid);
        vm.serializeUint(key, "blockNumber", block.number);
        vm.serializeAddress(key, "usdr", address(usdr));
        vm.serializeAddress(key, "faucet", address(faucet));
        vm.serializeAddress(key, "vault", address(vault));
        vm.serializeAddress(key, "vaultFaucet", vaults[0]);
        vm.serializeAddress(key, "vaultInsurance", vaults[1]);
        vm.serializeAddress(key, "vaultTreasury", vaults[2]);
        vm.serializeAddress(key, "vaultTeam", vaults[3]);
        string memory json = vm.serializeAddress(key, "vaultLiquidity", vaults[4]);

        string memory chainName = _chainName(block.chainid);
        string memory path = string.concat("../deployments/", chainName, ".json");
        vm.writeJson(json, path);
        console2.log("Wrote deployment JSON:", path);
    }

    function _chainName(uint256 chainid) internal pure returns (string memory) {
        if (chainid == 421_614) return "arbitrum-sepolia";
        if (chainid == 11_155_111) return "sepolia";
        if (chainid == 31_337) return "localhost";
        return "unknown";
    }
}
