# Domain Context

## Exchange TPS Collector

The Exchange TPS Collector records whole-exchange throughput in fixed time buckets. It is separate from latency benchmark samples and should not store raw exchange payloads.

## Deployment Topology

The Deployment Topology is the repo-owned description of long-running service
commands, stores, flags, and validation checks. It is the source of truth for
process supervisors and should catch binary/flag drift before a service is
deployed.

## Process Event

A Process Event is a durable JSONL lifecycle record emitted by long-running
benchmark services when they start, stop, retry, or exit. It is separate from
benchmark samples and exists to explain service health after journal retention or
restart loops erase useful context.

## Continuous Run Supervisor

The Continuous Run Supervisor keeps a live benchmark collector process alive
across retryable run, cleanup, retention, or write failures. It records retry
events and backs off instead of turning a transient Venue Account issue into
silent service downtime.

## Throughput Bucket

A Throughput Bucket is a compact aggregate for one venue and one UTC bucket start. Counts are stored as integers; TPS is derived as `tx / bucket_seconds`.

## Source Quality

Source Quality describes how a venue's Throughput Bucket was produced. Block-derived data is exact for the observed stream. Provider-reported data is accepted from a venue or third-party metric endpoint and may be converted into integer counts.

## Funding Monitor

The Funding Monitor is a separate process that reads Venue Account collateral and
submits Arbitrum USDC deposits when configured thresholds are breached. It is not
part of the measured benchmark path.

## Benchmark Funding Wallet

The Benchmark Funding Wallet is the Arbitrum wallet that holds USDC for automatic
top-ups. Venue-specific deposit routes may credit a different Venue Account
address, but transactions are funded from this wallet unless a route explicitly
overrides the signer.

## Venue Account

A Venue Account is the exchange account whose collateral, positions, orders, and
API keys are used by benchmark runs. Low-balance funding decisions are made per
Venue Account.

## Lighter API Key Role

A Lighter API Key Role selects the maker, taker, or default trading API key for
a Lighter Venue Account. Order submission, cleanup, reconciliation, and run locks
must resolve the same role so each live runner owns one nonce stream.
