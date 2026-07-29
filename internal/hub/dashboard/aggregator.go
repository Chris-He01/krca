package dashboard

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"knsight-go/internal/hub/store"
)

// Supported time ranges for dashboard aggregation.
var Ranges = []string{"1h", "24h", "7d", "30d", "all"}

// RangeToDuration converts a range key to a time.Duration. "all" returns 0 (no since filter).
func RangeToDuration(r string) time.Duration {
	switch r {
	case "1h":
		return 1 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	case "all":
		return 0
	default:
		return 24 * time.Hour
	}
}

// bucketWidth returns the time bucket width for trend charts.
func bucketWidth(rangeKey string) time.Duration {
	switch rangeKey {
	case "1h":
		return 5 * time.Minute
	case "24h":
		return 1 * time.Hour
	case "7d":
		return 24 * time.Hour
	case "30d":
		return 24 * time.Hour
	case "all":
		return 7 * 24 * time.Hour
	default:
		return 1 * time.Hour
	}
}

// RedisClient is a minimal interface for writing dashboard snapshots to Redis.
// Matches the kedis.Kedis methods we need. Pass nil to use in-memory cache only.
type RedisClient interface {
	Set(key string, value interface{}, expiration time.Duration) error
	Get(key string) (string, error)
}

// Aggregator periodically computes dashboard metrics from session data.
type Aggregator struct {
	store   store.SessionStore
	sceneID string
	redis   RedisClient // nil = in-memory only
	prefix  string      // Redis key prefix, e.g. "prod"
	cache   sync.Map    // rangeKey -> *DashboardData (always used as primary read cache)
	stopCh  chan struct{}
}

// NewAggregator creates a Dashboard Aggregator with in-memory cache only.
func NewAggregator(s store.SessionStore, sceneID string) *Aggregator {
	return &Aggregator{
		store:   s,
		sceneID: sceneID,
		stopCh:  make(chan struct{}),
	}
}

// NewAggregatorWithRedis creates a Dashboard Aggregator that also writes snapshots to Redis.
func NewAggregatorWithRedis(s store.SessionStore, sceneID string, redis RedisClient, prefix string) *Aggregator {
	return &Aggregator{
		store:   s,
		sceneID: sceneID,
		redis:   redis,
		prefix:  prefix,
		stopCh:  make(chan struct{}),
	}
}

// Start runs the aggregation loops. Call once after creation.
// On cold start: first tries to load cached snapshots from Redis (instant),
// then computes fresh aggregation from SessionStore (may take a moment for large datasets).
func (a *Aggregator) Start() {
	// Try to restore from Redis first (fast, avoids empty dashboard on restart)
	a.tryLoadFromRedis()

	// Probe the underlying store so operators can see backfill is wired up:
	// log how many historical sessions we'll consider and the most recent update time.
	a.logBackfillProbe()

	// Cold start: aggregate all ranges from SessionStore.
	// When sceneID is empty (普通版 / hub.prod.yaml)，会把所有历史 session 都聚合到
	// 看板缓存里——这就是"启动时把 Redis 已有数据转成看板可见数据"的实际机制。
	log.Printf("[dashboard] cold start aggregation for scene=%q", a.sceneID)
	for _, r := range Ranges {
		a.aggregate(r)
	}
	log.Printf("[dashboard] cold start complete")

	go a.loop(5*time.Minute, []string{"1h"})
	go a.loop(1*time.Hour, []string{"24h"})
	go a.loop(6*time.Hour, []string{"7d", "30d", "all"})
}

// logBackfillProbe scans the SessionStore once to surface how many historical
// sessions exist for the configured scene and when the latest update was.
// Empty sceneID means "all sessions"，which matches the default hub.prod.yaml
// deployment where sessions have no scene_id stamped in metadata.
func (a *Aggregator) logBackfillProbe() {
	ctx := context.Background()
	sessions, err := a.store.ListSessions(ctx, "", 10000, 0, a.sceneID, nil)
	if err != nil {
		log.Printf("[dashboard] backfill probe error: %v", err)
		return
	}
	if len(sessions) == 0 {
		log.Printf("[dashboard] backfill probe: 0 sessions for scene=%q (nothing to show)", a.sceneID)
		return
	}
	var latest time.Time
	for _, s := range sessions {
		if s.UpdatedAt.After(latest) {
			latest = s.UpdatedAt
		}
	}
	log.Printf("[dashboard] backfill probe: %d sessions for scene=%q, latest update=%s",
		len(sessions), a.sceneID, latest.Format(time.RFC3339))
}

// Stop stops all aggregation loops.
func (a *Aggregator) Stop() {
	close(a.stopCh)
}

// Get returns the cached DashboardData for the given range, or nil if not yet computed.
func (a *Aggregator) Get(rangeKey string) *DashboardData {
	v, ok := a.cache.Load(rangeKey)
	if !ok {
		return nil
	}
	return v.(*DashboardData)
}

func (a *Aggregator) loop(interval time.Duration, ranges []string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for _, r := range ranges {
				a.aggregate(r)
			}
		case <-a.stopCh:
			return
		}
	}
}

func (a *Aggregator) aggregate(rangeKey string) {
	now := time.Now()
	dur := RangeToDuration(rangeKey)

	var since *time.Time
	if dur > 0 {
		t := now.Add(-dur)
		since = &t
	}

	ctx := context.Background()
	sessions, err := a.store.ListSessions(ctx, "", 10000, 0, a.sceneID, since)
	if err != nil {
		log.Printf("[dashboard] aggregate %s error: %v", rangeKey, err)
		return
	}

	data := a.compute(sessions, rangeKey, now)

	// Compute delta: previous period
	if dur > 0 {
		prevEnd := now.Add(-dur)
		prevStart := prevEnd.Add(-dur)
		prevSessions, err := a.store.ListSessions(ctx, "", 10000, 0, a.sceneID, &prevStart)
		if err == nil {
			// Filter to [prevStart, prevEnd)
			var filtered []*store.Session
			for _, s := range prevSessions {
				if s.CreatedAt.Before(prevEnd) {
					filtered = append(filtered, s)
				}
			}
			prev := a.compute(filtered, rangeKey, prevEnd)
			data.SessionCountDelta = data.SessionCount - prev.SessionCount
			data.UniqueUsersDelta = data.UniqueUsers - prev.UniqueUsers
			data.DiagnosedCountDelta = data.DiagnosedCount - prev.DiagnosedCount
			data.TokensDelta = data.TotalTokens - prev.TotalTokens
		}
	}

	a.cache.Store(rangeKey, data)

	// Write to Redis if available
	if a.redis != nil {
		rkey := a.redisKey(rangeKey)
		ttl := a.redisTTL(rangeKey)
		jsonBytes, err := json.Marshal(data)
		if err == nil {
			if err := a.redis.Set(rkey, string(jsonBytes), ttl); err != nil {
				log.Printf("[dashboard] redis write %s error: %v", rkey, err)
			}
		}
	}

	log.Printf("[dashboard] aggregated %s: sessions=%d users=%d diagnosed=%d",
		rangeKey, data.SessionCount, data.UniqueUsers, data.DiagnosedCount)
}

func (a *Aggregator) redisKey(rangeKey string) string {
	p := a.prefix
	if p != "" {
		p += ":"
	}
	scene := a.sceneID
	if scene == "" {
		scene = "_all" // 兼容普通版部署（hub.prod.yaml 无 scene_id），避免 key 出现双冒号
	}
	return p + "knsight:dash:" + scene + ":" + rangeKey
}

func (a *Aggregator) redisTTL(rangeKey string) time.Duration {
	switch rangeKey {
	case "1h":
		return 10 * time.Minute
	case "24h":
		return 2 * time.Hour
	default: // 7d, 30d, all
		return 12 * time.Hour
	}
}

// tryLoadFromRedis attempts to populate the in-memory cache from Redis on cold start.
// This avoids showing empty data if the server restarts while Redis has recent snapshots.
func (a *Aggregator) tryLoadFromRedis() {
	if a.redis == nil {
		return
	}
	for _, r := range Ranges {
		rkey := a.redisKey(r)
		val, err := a.redis.Get(rkey)
		if err != nil || val == "" {
			continue
		}
		var data DashboardData
		if err := json.Unmarshal([]byte(val), &data); err != nil {
			continue
		}
		a.cache.Store(r, &data)
		log.Printf("[dashboard] loaded %s from redis (sessions=%d, updated=%s)",
			r, data.SessionCount, data.LastUpdated.Format("15:04:05"))
	}
}

func (a *Aggregator) compute(sessions []*store.Session, rangeKey string, now time.Time) *DashboardData {
	d := &DashboardData{
		Range:       rangeKey,
		LastUpdated: now,
	}
	if len(sessions) == 0 {
		return d
	}

	bw := bucketWidth(rangeKey)
	userSet := make(map[string]bool)
	userTokens := make(map[string]int64)
	userCounts := make(map[string]int)
	typeCounter := make(map[string]int)
	stageCounter := make(map[string]int)
	confCounter := make(map[string]int)
	suspectCounter := make(map[string]*SuspectCount)
	activityBuckets := make(map[string]*ActivityBucket)
	trendBuckets := make(map[string]map[string]int)
	var totalDuration float64

	// 数据质量阈值：CreatedAt 早于 2024-01-01 视为坏数据（多半是 Go 零值
	// 0001-01-01），会让 avg_duration_sec 飙到几亿、activity_trend 出现一个
	// 远古时间点的桶。直接跳过，不计入聚合。
	sanityCutoff := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, s := range sessions {
		// Skip sessions with corrupted timestamps (零值 CreatedAt)
		if s.CreatedAt.Before(sanityCutoff) {
			continue
		}

		d.SessionCount++
		userSet[s.UserID] = true

		meta := parseMeta(s.Metadata)

		// Duration — 兜底夹紧到 [0, 24h]，避免坏 UpdatedAt 拉高 avg_duration_sec。
		// 真实诊断通常 < 10 分钟，超过 1 天的肯定是数据问题。
		dur := s.UpdatedAt.Sub(s.CreatedAt).Seconds()
		if dur < 0 {
			dur = 0
		}
		const maxDur = 24 * 60 * 60
		if dur > maxDur {
			dur = maxDur
		}
		totalDuration += dur

		// Tokens
		if meta.TokenUsage > 0 {
			d.TotalTokens += meta.TokenUsage
			userTokens[s.UserID] += meta.TokenUsage
		}
		userCounts[s.UserID]++

		// Diagnosed — 有明确的 conclusion_stage（socsci JSON 解析或新版 enrichSessionStatus
		// 写入），或者 token_usage > 0（LLM 实际回复过）都算完成。后者覆盖了在 117c84e 之后
		// 没有打 conclusion_stage 标记但已经成功跑过的历史 session。
		isDiagnosed := meta.ConclusionStage != "" || meta.TokenUsage > 0
		if isDiagnosed {
			d.DiagnosedCount++
		}

		// Type distribution — 优先用 metadata.interference_type（socsci 场景），
		// 否则按 agent_type 兜底分类（普通版 InsightSupervisor 不写 interference_type）。
		itype := meta.InterferenceType
		if itype == "" {
			itype = classifyByAgentType(s.AgentType, meta.SceneID)
		}
		typeCounter[itype]++

		// Stage
		stage := classifyStage(meta.ConclusionStage)
		if stage != "" {
			stageCounter[stage]++
		}

		// Confidence — 同样应用 token_usage 兜底：跑过的 session 即使没显式 conclusion_confidence
		// 也按 HIGH 计入，避免看板永远显示满坑 PENDING。
		conf := meta.ConclusionConfidence
		if conf == "" {
			if isDiagnosed {
				conf = "HIGH"
			} else {
				conf = "PENDING"
			}
		}
		confCounter[conf]++

		// Suspect ranking (from top_suspects in output)
		for _, sus := range meta.TopSuspects {
			key := sus.Pod
			if key == "" {
				continue
			}
			if _, ok := suspectCounter[key]; !ok {
				suspectCounter[key] = &SuspectCount{Pod: sus.Pod, KSN: sus.KSN}
			}
			suspectCounter[key].Count++
		}

		// Activity trend bucket
		bucketTime := s.CreatedAt.Truncate(bw).Format(time.RFC3339)
		if ab, ok := activityBuckets[bucketTime]; ok {
			ab.Sessions++
			ab.users[s.UserID] = true
		} else {
			activityBuckets[bucketTime] = &ActivityBucket{
				Time:     bucketTime,
				Sessions: 1,
				users:    map[string]bool{s.UserID: true},
			}
		}

		// Type trend bucket
		if _, ok := trendBuckets[bucketTime]; !ok {
			trendBuckets[bucketTime] = make(map[string]int)
		}
		trendBuckets[bucketTime][itype]++
	}

	d.UniqueUsers = len(userSet)
	if d.SessionCount > 0 {
		d.AvgDurationSec = totalDuration / float64(d.SessionCount)
	}

	// DiagStats
	d.DiagStats.Total = d.SessionCount
	d.DiagStats.RulesClosed = stageCounter["S1"] + stageCounter["S2"] + stageCounter["S3"] + stageCounter["S4"]
	d.DiagStats.LLMClosed = stageCounter["S5"]
	d.DiagStats.Pending = d.SessionCount - d.DiagnosedCount

	// Type distribution
	for t, c := range typeCounter {
		d.TypeDistribution = append(d.TypeDistribution, TypeCount{Type: t, Count: c})
	}
	sort.Slice(d.TypeDistribution, func(i, j int) bool {
		return d.TypeDistribution[i].Count > d.TypeDistribution[j].Count
	})

	// Suspect ranking top 10
	for _, sc := range suspectCounter {
		d.SuspectRanking = append(d.SuspectRanking, *sc)
	}
	sort.Slice(d.SuspectRanking, func(i, j int) bool {
		return d.SuspectRanking[i].Count > d.SuspectRanking[j].Count
	})
	if len(d.SuspectRanking) > 10 {
		d.SuspectRanking = d.SuspectRanking[:10]
	}

	// Activity trend
	for _, ab := range activityBuckets {
		ab.UV = len(ab.users)
		d.ActivityTrend = append(d.ActivityTrend, *ab)
	}
	sort.Slice(d.ActivityTrend, func(i, j int) bool {
		return d.ActivityTrend[i].Time < d.ActivityTrend[j].Time
	})

	// Type trend
	for t, types := range trendBuckets {
		d.TypeTrend = append(d.TypeTrend, TypeTrendBucket{Time: t, Types: types})
	}
	sort.Slice(d.TypeTrend, func(i, j int) bool {
		return d.TypeTrend[i].Time < d.TypeTrend[j].Time
	})

	// Pipeline funnel
	total := float64(d.SessionCount)
	for _, stage := range []string{"S1", "S2", "S3", "S4", "S5"} {
		cnt := stageCounter[stage]
		pct := 0.0
		if total > 0 {
			pct = float64(cnt) / total * 100
		}
		d.PipelineFunnel = append(d.PipelineFunnel, StageCount{Stage: stage, Count: cnt, Pct: pct})
	}

	// Confidence distribution
	for _, level := range []string{"HIGH", "MEDIUM", "LOW", "PENDING"} {
		d.ConfidenceDist = append(d.ConfidenceDist, ConfidenceCount{Level: level, Count: confCounter[level]})
	}

	// Token top users
	type ut struct {
		uid    string
		tokens int64
	}
	var uts []ut
	for uid, t := range userTokens {
		uts = append(uts, ut{uid, t})
	}
	sort.Slice(uts, func(i, j int) bool { return uts[i].tokens > uts[j].tokens })
	for _, u := range uts {
		d.TokenTopUsers = append(d.TokenTopUsers, UserTokenStat{
			UserID: u.uid,
			Tokens: u.tokens,
			Count:  userCounts[u.uid],
		})
	}

	return d
}

// classifyByAgentType returns a human-readable type label when a session has
// no metadata.interference_type. Plain InsightSupervisor sessions (普通版
// hub.prod.yaml) get "GENERAL"; scene-mode sessions are labeled by their
// scene id so 看板"诊断类型分布"不会全部归到 UNKNOWN。
func classifyByAgentType(agentType, sceneID string) string {
	if sceneID != "" {
		return strings.ToUpper(sceneID)
	}
	if at := strings.TrimSpace(agentType); at != "" {
		// "scene/socsci-interference" → "SOCSCI-INTERFERENCE"
		if rest, ok := strings.CutPrefix(at, "scene/"); ok && rest != "" {
			return strings.ToUpper(rest)
		}
		// "insight" → "GENERAL"（默认通用诊断）
		if at == "insight" {
			return "GENERAL"
		}
		return strings.ToUpper(at)
	}
	return "UNKNOWN"
}

func classifyStage(stage string) string {
	if stage == "" || stage == "completed" {
		return ""
	}
	if strings.HasPrefix(stage, "stage1") {
		return "S1"
	}
	if strings.HasPrefix(stage, "stage2") {
		return "S2"
	}
	if strings.HasPrefix(stage, "stage3") {
		return "S3"
	}
	if strings.HasPrefix(stage, "stage4") {
		return "S4"
	}
	return ""
}

// sessionMeta represents parsed metadata fields from session.metadata JSON.
type sessionMeta struct {
	SceneID              string       `json:"scene_id"`
	InterferenceType     string       `json:"interference_type"`
	ConclusionStage      string       `json:"conclusion_stage"`
	ConclusionConfidence string       `json:"conclusion_confidence"`
	TokenUsage           int64        `json:"token_usage"`
	TopSuspects          []suspectRef `json:"top_suspects"`
}

type suspectRef struct {
	Pod string `json:"candidate_pod"`
	KSN string `json:"candidate_ksn"`
}

func parseMeta(raw string) sessionMeta {
	var m sessionMeta
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &m)
	}
	return m
}
