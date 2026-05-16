// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Test} from "forge-std/Test.sol";
import {USDR} from "../src/USDR.sol";
import {GameVault} from "../src/GameVault.sol";

contract GameVaultTest is Test {
    address constant FAUCET_VAULT = address(0x1111);
    address constant INSURANCE = address(0x2222);
    address constant TREASURY = address(0x3333);
    address constant TEAM = address(0x4444);
    address constant LIQUIDITY = address(0x5555);

    USDR usdr;
    GameVault vault;

    bytes32 constant LEVEL_BTC_MED = keccak256("D-312-BTC");

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
        vault = new GameVault(usdr);
    }

    function _fund(address player, uint256 amount) internal {
        vm.prank(FAUCET_VAULT);
        usdr.transfer(player, amount);
    }

    function test_DepositSucceeds_EmitsSessionStarted() public {
        address alice = makeAddr("alice");
        _fund(alice, 1000 * 1e18);

        vm.startPrank(alice);
        usdr.approve(address(vault), 500 * 1e18);

        // sessionId = keccak256(alice, level, nonce=1)
        bytes32 expectedId = keccak256(abi.encode(alice, LEVEL_BTC_MED, uint256(1)));

        vm.expectEmit(true, true, true, true, address(vault));
        emit GameVault.SessionStarted(alice, LEVEL_BTC_MED, expectedId, 500 * 1e18, 1, block.number);

        bytes32 sid = vault.deposit(LEVEL_BTC_MED, 500 * 1e18);
        vm.stopPrank();

        assertEq(sid, expectedId);
        assertEq(usdr.balanceOf(alice), 500 * 1e18);
        assertEq(usdr.balanceOf(address(vault)), 500 * 1e18);
        assertEq(vault.sessionBalance(sid), 500 * 1e18);
        assertEq(vault.sessionLevel(sid), LEVEL_BTC_MED);
        assertEq(vault.sessionOwner(sid), alice);
        assertEq(vault.depositNonce(alice), 1);
    }

    function test_GetSessionReturnsTuple() public {
        address alice = makeAddr("alice");
        _fund(alice, 1000 * 1e18);
        vm.startPrank(alice);
        usdr.approve(address(vault), 500 * 1e18);
        bytes32 sid = vault.deposit(LEVEL_BTC_MED, 500 * 1e18);
        vm.stopPrank();

        (address player, bytes32 lvl, uint256 amt) = vault.getSession(sid);
        assertEq(player, alice);
        assertEq(lvl, LEVEL_BTC_MED);
        assertEq(amt, 500 * 1e18);
    }

    function test_TwoDepositsYieldDistinctSessionIds() public {
        address alice = makeAddr("alice");
        _fund(alice, 2000 * 1e18);

        vm.startPrank(alice);
        usdr.approve(address(vault), 1000 * 1e18);
        bytes32 s1 = vault.deposit(LEVEL_BTC_MED, 500 * 1e18);
        bytes32 s2 = vault.deposit(LEVEL_BTC_MED, 500 * 1e18);
        vm.stopPrank();

        assertTrue(s1 != s2, "sessionIds must differ");
        assertEq(vault.depositNonce(alice), 2);
    }

    function test_RevertWhen_AmountBelowMin() public {
        address alice = makeAddr("alice");
        _fund(alice, 100 * 1e18);
        vm.startPrank(alice);
        usdr.approve(address(vault), 50 * 1e18);
        vm.expectRevert(
            abi.encodeWithSelector(
                GameVault.AmountBelowMin.selector, 50 * 1e18, vault.MIN_DEPOSIT()
            )
        );
        vault.deposit(LEVEL_BTC_MED, 50 * 1e18);
        vm.stopPrank();
    }

    function test_RevertWhen_AmountAboveMax() public {
        address alice = makeAddr("alice");
        _fund(alice, 100_000 * 1e18);
        vm.startPrank(alice);
        usdr.approve(address(vault), 60_000 * 1e18);
        vm.expectRevert(
            abi.encodeWithSelector(
                GameVault.AmountAboveMax.selector, 60_000 * 1e18, vault.MAX_DEPOSIT()
            )
        );
        vault.deposit(LEVEL_BTC_MED, 60_000 * 1e18);
        vm.stopPrank();
    }

    function test_RevertWhen_LevelIdZero() public {
        address alice = makeAddr("alice");
        _fund(alice, 1000 * 1e18);
        vm.startPrank(alice);
        usdr.approve(address(vault), 500 * 1e18);
        vm.expectRevert(GameVault.ZeroLevelId.selector);
        vault.deposit(bytes32(0), 500 * 1e18);
        vm.stopPrank();
    }

    function test_RevertWhen_NoApproval() public {
        address alice = makeAddr("alice");
        _fund(alice, 1000 * 1e18);
        vm.prank(alice);
        // OZ ERC20: ERC20InsufficientAllowance(spender, allowance, needed)
        vm.expectRevert(); // selector check is brittle across OZ versions; just expect any revert
        vault.deposit(LEVEL_BTC_MED, 500 * 1e18);
    }

    function test_RevertWhen_InsufficientBalance() public {
        address alice = makeAddr("alice");
        _fund(alice, 100 * 1e18);
        vm.startPrank(alice);
        usdr.approve(address(vault), 500 * 1e18);
        vm.expectRevert();
        vault.deposit(LEVEL_BTC_MED, 500 * 1e18);
        vm.stopPrank();
    }

    function test_RevertWhen_Withdraw() public {
        vm.expectRevert(GameVault.WithdrawNotImplemented.selector);
        vault.withdraw(bytes32(uint256(1)), bytes(""));
    }

    function test_RevertWhen_ConstructedWithZeroToken() public {
        vm.expectRevert(GameVault.ZeroAddressToken.selector);
        new GameVault(USDR(address(0)));
    }
}
