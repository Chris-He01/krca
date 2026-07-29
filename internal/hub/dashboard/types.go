package dashboard

import "time"

// DashboardData is the aggregated snapshot for a single time range.
type DashboardData struct {
	Range string `json:"range"` // "1h" / "24h" / "7d" / "30d" / "all"

	// User analytics
	SessionCount   int             `json:"session_count"`
	UniqueUsers    int             `json:"unique_users"`
	AvgDurationSec float64         `json:"avg_duration_sec"`
	TotalTokens    int64           `json:"total_tokens"`
	DiagnosedCount int             `json:"diagnosed_count"`
	ActivityTrend  []ActivityBucket `json:"activity_trend"`
	TokenTopUsers  []UserTokenStat `json:"token_top_users"`

	// Diagnostic analytics
	DiagStats        DiagStats         `json:"diag_stats"`
	TypeDistribution []TypeCount       `json:"type_distribution"`
	SuspectRanking   []SuspectCount    `json:"suspect_ranking"`
	TypeTrend        []TypeTrendBucket `json:"type_trend"`
	PipelineFunnel   []StageCount      `json:"pipeline_funnel"`
	ConfidenceDist   []ConfidenceCount `json:"confidence_dist"`

	// Delta vs previous period
	SessionCountDelta   int   `json:"session_count_delta"`
	UniqueUsersDelta    int   `json:"unique_users_delta"`
	DiagnosedCountDelta int   `json:"diagnosed_count_delta"`
	TokensDelta         int64 `json:"tokens_delta"`

	// Metadata
	LastUpdated time.Time `json:"last_updated"`
}

// ActivityBucket is a time bucket for user activity trend charts.
type ActivityBucket struct {
	Time     string `json:"time"`
	UV       int    `json:"uv"`
	Sessions int    `json:"sessions"`

	users map[string]bool `json:"-"` // internal: track unique users per bucket
}

type UserTokenStat struct {
	UserID string `json:"user_id"`
	Tokens int64  `json:"tokens"`
	Count  int    `json:"count"`
}

type DiagStats struct {
	Total       int `json:"total"`
	RulesClosed int `json:"rules_closed"`
	LLMClosed   int `json:"llm_closed"`
	Pending     int `json:"pending"`
}

type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type SuspectCount struct {
	Pod   string `json:"pod"`
	KSN   string `json:"ksn"`
	Count int    `json:"count"`
}

type TypeTrendBucket struct {
	Time  string         `json:"time"`
	Types map[string]int `json:"types"`
}

type StageCount struct {
	Stage string  `json:"stage"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
}

type ConfidenceCount struct {
	Level string `json:"level"`
	Count int    `json:"count"`
}
