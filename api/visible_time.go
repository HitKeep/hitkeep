package api

// VisibleTimePage summarizes tracked pageviews for one path.
type VisibleTimePage struct {
	Path          string   `json:"path"`
	AvgVisibleMS  *float64 `json:"avg_visible_ms"`
	MeasuredViews int64    `json:"measured_views"`
	TrackedViews  int64    `json:"tracked_views"`
}
