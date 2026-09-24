# Funding Monitor

`perps-bench funding monitor` runs outside benchmark timing and tops up configured
venue accounts when their existing balance adapters report low collateral.

Start in dry-run mode:

```sh
go run ./cmd/perps-bench funding monitor \
  --config examples/funding.json \
  --once
```

Live deposits require both `dry_run=false` in config or `--dry-run=false`, and
`--confirm-live`.

## Safety Model

- Funding is a separate long-running service, not part of `run-continuous` or
  `run-coordinated`.
- Balance reads reuse the venue cost/balance adapters.
- Deposit decisions are recorded in `state_path` and respect per-account
  cooldowns.
- Config values name secret environment variables; private keys must not be
  placed directly in JSON.
- Arbitrum is allowlisted by chain ID and the native Arbitrum USDC contract is
  the default token.

## Deposit Routes

`evm_usdc_transfer` sends native Arbitrum USDC from the configured signing
wallet to a destination address. Hyperliquid deposits use this route with the
official bridge address. Hyperliquid credits the sender, so the signing wallet
must be the Hyperliquid account wallet being topped up.

`lighter_cctp_intent` creates a Lighter Arbitrum CCTP intent address for the
configured L1 wallet, then sends native Arbitrum USDC from the benchmark funding
wallet to that intent address. Set `from_address_env` to the L1 address that
should be credited. The `lighter_free` account is intentionally configured with
`LIGHTER_FREE_L1_ADDRESS`, because that account lives under a separate L1
address.

`command` is the extension route for venues whose deposit flow is bridge/API
specific. The monitor sends the `DepositPlan` JSON on stdin and expects a
`DepositResult` JSON on stdout.

`extended_rhino_bridge` uses Extended's EVM bridge flow: fetch bridge config,
quote an Arbitrum-to-Starknet USDC deposit, commit the quote, then approve and
call Rhino's `depositWithId` on Arbitrum. The quote fee must not exceed
`max_fee_usdc`.

`aster_treasury_deposit` approves and deposits an allowlisted Arbitrum ERC-20
token into Aster's Arbitrum treasury contract. The example config uses native
Arbitrum USDC because Aster's deposit UI/docs support `ARB-USDC`. The signing
wallet must be the Aster user wallet; the API wallet key cannot receive deposits
by itself.

## Current Venue Coverage

- Hyperliquid: direct Arbitrum native USDC transfer to the bridge.
- Lighter / Lighter free: Arbitrum CCTP intent address plus native USDC transfer.
- Extended: Arbitrum native USDC through Extended's Rhino bridge flow.
- Aster: Arbitrum native USDC through Aster's treasury contract.
- Nado: balance monitoring can be configured today, but live deposits should use
  a `command` adapter or wait for direct route support. Nado needs explicit
  confirmation that the production collateral path accepts Arbitrum USDC
  directly.
