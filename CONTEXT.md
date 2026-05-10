# Domain Context

## Exchange TPS Collector

The Exchange TPS Collector records whole-exchange throughput in fixed time buckets. It is separate from latency benchmark samples and should not store raw exchange payloads.

## Throughput Bucket

A Throughput Bucket is a compact aggregate for one venue and one UTC bucket start. Counts are stored as integers; TPS is derived as `tx / bucket_seconds`.

## Source Quality

Source Quality describes how a venue's Throughput Bucket was produced. Block-derived data is exact for the observed stream. Provider-reported data is accepted from a venue or third-party metric endpoint and may be converted into integer counts.
