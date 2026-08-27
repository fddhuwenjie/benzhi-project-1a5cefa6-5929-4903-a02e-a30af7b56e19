package domain

var lifecycleOrder = []Status{StatusDraft, StatusProtocolFrozen, StatusCollecting, StatusEvaluationFailed, StatusCorrecting, StatusReperforming, StatusReviewPending, StatusQualified, StatusRejected}

func LifecycleStatuses() []Status { return append([]Status(nil), lifecycleOrder...) }
func CanWrite(status Status) bool { return status != StatusQualified && status != StatusRejected }
func IsKnownStatus(status Status) bool {
	for _, candidate := range lifecycleOrder {
		if candidate == status {
			return true
		}
	}
	return false
}
