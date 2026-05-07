package bench

type LatencySemantics struct {
	RawNetworkNS             int64
	ConfirmationNS           int64
	NetworkAdjustedNS        int64
	NetworkFloorNS           int64
	NetworkAdjustedCleanupNS int64
	SpeedBumpNS              int64
	SpeedBumpSource          string
	CancelNS                 int64
	HasNetworkFloor          bool
	HasCancel                bool
}

func LatencyForSample(sample Sample) LatencySemantics {
	raw := RawNetworkNS(sample)
	confirmation := AdjustedNetworkNS(sample)
	semantics := LatencySemantics{
		RawNetworkNS:      raw,
		ConfirmationNS:    confirmation,
		SpeedBumpNS:       SpeedBumpNS(sample),
		SpeedBumpSource:   SpeedBumpSource(sample),
		NetworkFloorNS:    sample.NetworkFloorNS,
		HasNetworkFloor:   sample.NetworkFloorNS > 0,
		NetworkAdjustedNS: confirmation,
	}
	if adjusted, ok := NetworkAdjustedNetworkNS(sample); ok {
		semantics.NetworkAdjustedNS = adjusted
	}
	if cancelNS, ok := CancelLatencyNS(sample); ok {
		semantics.CancelNS = cancelNS
		semantics.HasCancel = true
		if sample.NetworkFloorNS > 0 {
			semantics.NetworkAdjustedCleanupNS = ClampLatencyNS(cancelNS - sample.NetworkFloorNS)
		}
	}
	return semantics
}

func AdjustForSpeedBumpNS(rawNS int64, speedBumpNS int64) int64 {
	return ClampLatencyNS(rawNS - speedBumpNS)
}

func ClampLatencyNS(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}
