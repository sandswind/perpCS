// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Test} from "forge-std/Test.sol";
import {USDR} from "../src/USDR.sol";
import {USDRFaucet} from "../src/USDRFaucet.sol";

contract USDRFaucetTest is Test {
    address constant FAUCET_VAULT = address(0x1111);
    address constant INSURANCE = address(0x2222);
    address constant TREASURY = address(0x3333);
    address constant TEAM = address(0x4444);
    address constant LIQUIDITY = address(0x5555);

    USDR usdr;
    USDRFaucet faucet;

    function setUp() public {
        address[5] memory vs = [FAUCET_VAULT, INSURANCE, TREASURY, TEAM, LIQUIDITY];
        uint256[5] memory amts = [
            uint256(60_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000),
            uint256(10_000_000_000)
        ];
        usdr = new USDR(vs, amts);
        faucet = new USDRFaucet(usdr);

        // Fund the faucet from the FAUCET_VAULT (60B USDR).
        vm.prank(FAUCET_VAULT);
        usdr.transfer(address(faucet), 60_000_000_000 * 1e18);
    }

    function test_FaucetFunded60B() public view {
        assertEq(usdr.balanceOf(address(faucet)), 60_000_000_000 * 1e18);
        assertEq(faucet.remainingBalance(), 60_000_000_000 * 1e18);
    }

    function test_ClaimSucceedsOnce() public {
        address alice = makeAddr("alice");
        assertFalse(faucet.hasClaimed(alice));

        vm.expectEmit(true, false, false, true, address(faucet));
        emit USDRFaucet.Claimed(alice, 10_000 * 1e18, 10_000 * 1e18);

        vm.prank(alice);
        faucet.claim();

        assertEq(usdr.balanceOf(alice), 10_000 * 1e18);
        assertTrue(faucet.hasClaimed(alice));
        assertEq(faucet.totalClaimed(), 10_000 * 1e18);
    }

    function test_RevertWhen_DoubleClaimSameWallet() public {
        address alice = makeAddr("alice");
        vm.startPrank(alice);
        faucet.claim();
        vm.expectRevert(USDRFaucet.AlreadyClaimed.selector);
        faucet.claim();
        vm.stopPrank();
    }

    function test_DifferentWalletsCanEachClaimOnce() public {
        address alice = makeAddr("alice");
        address bob = makeAddr("bob");
        address carol = makeAddr("carol");

        vm.prank(alice);
        faucet.claim();
        vm.prank(bob);
        faucet.claim();
        vm.prank(carol);
        faucet.claim();

        assertEq(usdr.balanceOf(alice), 10_000 * 1e18);
        assertEq(usdr.balanceOf(bob), 10_000 * 1e18);
        assertEq(usdr.balanceOf(carol), 10_000 * 1e18);
        assertEq(faucet.totalClaimed(), 30_000 * 1e18);
    }

    function test_RevertWhen_FaucetEmpty() public {
        // Deploy a fresh faucet and fund it with less than CLAIM_AMOUNT.
        // (FAUCET_VAULT was already drained in setUp, so fund from INSURANCE.)
        USDRFaucet emptyFaucet = new USDRFaucet(usdr);
        vm.prank(INSURANCE);
        usdr.transfer(address(emptyFaucet), 1000 * 1e18);

        address alice = makeAddr("alice");
        vm.prank(alice);
        vm.expectRevert(
            abi.encodeWithSelector(
                USDRFaucet.InsufficientFaucetBalance.selector, 1000 * 1e18, 10_000 * 1e18
            )
        );
        emptyFaucet.claim();
    }

    function test_RevertWhen_ConstructedWithZeroToken() public {
        vm.expectRevert(USDRFaucet.ZeroAddressToken.selector);
        new USDRFaucet(USDR(address(0)));
    }

    /// @dev Fuzz: any number of unique wallets each get exactly 10k once.
    function testFuzz_NUniqueClaimers(uint8 n) public {
        n = uint8(bound(n, 1, 50));
        for (uint256 i = 0; i < n; i++) {
            address player = address(uint160(uint256(keccak256(abi.encode("p", i)))));
            // skip if address collides with an already-claimed one (edge case)
            if (faucet.hasClaimed(player)) continue;
            vm.prank(player);
            faucet.claim();
            assertEq(usdr.balanceOf(player), 10_000 * 1e18);
        }
    }
}
