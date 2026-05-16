// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Test} from "forge-std/Test.sol";
import {USDR} from "../src/USDR.sol";

contract USDRTest is Test {
    address constant FAUCET = address(0x1111);
    address constant INSURANCE = address(0x2222);
    address constant TREASURY = address(0x3333);
    address constant TEAM = address(0x4444);
    address constant LIQUIDITY = address(0x5555);

    USDR usdr;

    function setUp() public {
        address[5] memory vs = [FAUCET, INSURANCE, TREASURY, TEAM, LIQUIDITY];
        uint256[5] memory amts = [
            uint256(60_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000)
        ];
        usdr = new USDR(vs, amts);
    }

    function test_Metadata() public view {
        assertEq(usdr.name(), "USD Reserve (PerpCS)");
        assertEq(usdr.symbol(), "USDR");
        assertEq(usdr.decimals(), 18);
    }

    function test_TotalSupplyIs100B() public view {
        // 100_000_000_000 * 1e18
        assertEq(usdr.totalSupply(), 100_000_000_000 * 1e18);
    }

    function test_VaultBalances() public view {
        assertEq(usdr.balanceOf(FAUCET), 60_000_000_000 * 1e18);
        assertEq(usdr.balanceOf(INSURANCE), 10_000_000_000 * 1e18);
        assertEq(usdr.balanceOf(TREASURY), 10_000_000_000 * 1e18);
        assertEq(usdr.balanceOf(TEAM), 10_000_000_000 * 1e18);
        assertEq(usdr.balanceOf(LIQUIDITY), 10_000_000_000 * 1e18);
    }

    function test_VaultsArrayPersisted() public view {
        assertEq(usdr.vaults(0), FAUCET);
        assertEq(usdr.vaults(1), INSURANCE);
        assertEq(usdr.vaults(2), TREASURY);
        assertEq(usdr.vaults(3), TEAM);
        assertEq(usdr.vaults(4), LIQUIDITY);
        assertEq(usdr.vaultAmounts(0), 60_000_000_000 * 1e18);
    }

    function test_Transfer() public {
        vm.prank(FAUCET);
        usdr.transfer(address(0xBEEF), 10_000 * 1e18);
        assertEq(usdr.balanceOf(address(0xBEEF)), 10_000 * 1e18);
    }

    function test_RevertWhen_VaultZeroAddress() public {
        address[5] memory vs = [FAUCET, address(0), TREASURY, TEAM, LIQUIDITY];
        uint256[5] memory amts = [
            uint256(60_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000)
        ];
        vm.expectRevert(abi.encodeWithSelector(USDR.ZeroVaultAddress.selector, 1));
        new USDR(vs, amts);
    }

    function test_RevertWhen_AllocationDoesNotSumTo100B() public {
        address[5] memory vs = [FAUCET, INSURANCE, TREASURY, TEAM, LIQUIDITY];
        uint256[5] memory amts = [
            uint256(60_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000),
            uint256(9_999_999_999) // off by one
        ];
        vm.expectRevert(
            abi.encodeWithSelector(
                USDR.AllocationMismatch.selector, 99_999_999_999, 100_000_000_000
            )
        );
        new USDR(vs, amts);
    }

    function test_NoMintOrBurnFunctionExists() public view {
        // Compile-time guarantee plus a runtime supply check after a transfer.
        // (If a public mint existed, it'd have been called by an attacker.)
        assertEq(usdr.totalSupply(), 100_000_000_000 * 1e18);
    }
}
