package connector

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/beeper/ai-bridge/pkg/memory"
	"github.com/beeper/ai-bridge/pkg/textfs"
	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
)

const memorySnippetMaxChars = 700

type MemorySearchManager struct {
	client       *AIClient
	db           *dbutil.Database
	bridgeID     string
	loginID      string
	agentID      string
	cfg          *memory.ResolvedConfig
	provider     memory.EmbeddingProvider
	status       memory.ProviderStatus
	providerKey  string
	vectorDims   int
	ftsAvailable bool
	ftsError     string
	log          zerolog.Logger

	dirty             bool
	sessionWarm       map[string]struct{}
	watchTimer        *time.Timer
	sessionWatchTimer *time.Timer
	sessionWatchKey   string
	intervalOnce      sync.Once
	vectorConn        *sql.Conn
	vectorReady       bool
	vectorError       string
	batchEnabled      bool
	batchFailures     int
	batchLastError    string
	batchLastProvider string
	mu                sync.Mutex
}

type MemorySearchStatus struct {
	Provider            string
	Model               string
	Fallback            *memory.FallbackStatus
	Sources             []string
	ExtraPaths          []string
	VectorEnabled       bool
	VectorReady         bool
	VectorError         string
	FTSEnabled          bool
	FTSAvailable        bool
	CacheEnabled        bool
	CacheEntries        int
	FileCount           int
	ChunkCount          int
	BatchEnabled        bool
	BatchFailures       int
	BatchLastError      string
	BatchLastProvider   string
	BatchWait           bool
	BatchConcurrency    int
	BatchPollIntervalMs int
	BatchTimeoutMinutes int
}

var memoryManagerCache = struct {
	mu       sync.Mutex
	managers map[string]*MemorySearchManager
}{
	managers: make(map[string]*MemorySearchManager),
}

func getMemorySearchManager(client *AIClient, agentID string) (*MemorySearchManager, string) {
	if client == nil || client.connector == nil {
		return nil, "memory search unavailable"
	}
	cfg, err := resolveMemorySearchConfig(client, agentID)
	if err != nil || cfg == nil {
		if err != nil {
			return nil, err.Error()
		}
		return nil, "memory search disabled"
	}

	bridgeID := string(client.UserLogin.Bridge.DB.BridgeID)
	loginID := string(client.UserLogin.ID)
	if agentID == "" {
		agentID = "default"
	}

	cacheKey := memoryManagerCacheKey(bridgeID, loginID, agentID, cfg)

	memoryManagerCache.mu.Lock()
	defer memoryManagerCache.mu.Unlock()
	if existing := memoryManagerCache.managers[cacheKey]; existing != nil {
		return existing, ""
	}

	providerResult, err := buildMemoryProvider(client, cfg)
	if err != nil {
		return nil, err.Error()
	}

	manager := &MemorySearchManager{
		client:      client,
		db:          client.UserLogin.Bridge.DB.Database,
		bridgeID:    bridgeID,
		loginID:     loginID,
		agentID:     agentID,
		cfg:         cfg,
		provider:    providerResult.Provider,
		status:      providerResult.Status,
		providerKey: providerResult.ProviderKey,
		log:         client.log.With().Str("component", "memory").Logger(),
	}
	if hasSource(cfg.Sources, "memory") {
		manager.dirty = true
	}
	manager.batchEnabled = cfg.Remote.Batch.Enabled

	manager.ensureSchema(context.Background())
	manager.ensureVectorConn(context.Background())
	manager.ensureIntervalSync()
	memoryManagerCache.managers[cacheKey] = manager
	return manager, ""
}

func (m *MemorySearchManager) Status() memory.ProviderStatus {
	return m.status
}

func (m *MemorySearchManager) StatusDetails(ctx context.Context) (*MemorySearchStatus, error) {
	if m == nil {
		return nil, fmt.Errorf("memory search unavailable")
	}
	status := &MemorySearchStatus{
		Provider:            m.status.Provider,
		Model:               m.status.Model,
		Fallback:            m.status.Fallback,
		Sources:             append([]string{}, m.cfg.Sources...),
		ExtraPaths:          append([]string{}, m.cfg.ExtraPaths...),
		VectorEnabled:       m.cfg.Store.Vector.Enabled,
		VectorReady:         m.vectorReady,
		VectorError:         m.vectorError,
		FTSEnabled:          m.cfg.Query.Hybrid.Enabled,
		FTSAvailable:        m.ftsAvailable,
		CacheEnabled:        m.cfg.Cache.Enabled,
		BatchEnabled:        m.batchEnabled && (m.status.Provider == "openai" || m.status.Provider == "gemini"),
		BatchFailures:       m.batchFailures,
		BatchLastError:      m.batchLastError,
		BatchLastProvider:   m.batchLastProvider,
		BatchWait:           m.cfg.Remote.Batch.Wait,
		BatchConcurrency:    m.cfg.Remote.Batch.Concurrency,
		BatchPollIntervalMs: m.cfg.Remote.Batch.PollIntervalMs,
		BatchTimeoutMinutes: m.cfg.Remote.Batch.TimeoutMinutes,
	}

	if m.cfg.Cache.Enabled {
		row := m.db.QueryRow(ctx,
			`SELECT COUNT(*) FROM ai_memory_embedding_cache WHERE bridge_id=$1 AND provider=$2 AND model=$3 AND provider_key=$4`,
			m.bridgeID, m.status.Provider, m.status.Model, m.providerKey,
		)
		_ = row.Scan(&status.CacheEntries)
	}

	row := m.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM ai_memory_files WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3`,
		m.bridgeID, m.loginID, m.agentID,
	)
	_ = row.Scan(&status.FileCount)
	row = m.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM ai_memory_chunks WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3`,
		m.bridgeID, m.loginID, m.agentID,
	)
	_ = row.Scan(&status.ChunkCount)

	return status, nil
}

func (m *MemorySearchManager) Search(ctx context.Context, query string, opts memory.SearchOptions) ([]memory.SearchResult, error) {
	if m == nil {
		return nil, fmt.Errorf("memory search unavailable")
	}
	m.warmSession(ctx, opts.SessionKey)
	if m.cfg.Sync.OnSearch {
		if err := m.sync(ctx, opts.SessionKey, false); err != nil {
			m.log.Warn().Err(err).Msg("memory sync failed on search")
		}
	}

	cleaned := strings.TrimSpace(query)
	if cleaned == "" {
		return []memory.SearchResult{}, nil
	}

	maxResults := m.cfg.Query.MaxResults
	if opts.MaxResults > 0 {
		maxResults = opts.MaxResults
	}
	if maxResults <= 0 {
		maxResults = memory.DefaultMaxResults
	}

	minScore := m.cfg.Query.MinScore
	if !math.IsNaN(opts.MinScore) {
		minScore = opts.MinScore
	}

	candidates := maxResults
	if m.cfg.Query.Hybrid.CandidateMultiplier > 0 {
		candidates = maxResults * m.cfg.Query.Hybrid.CandidateMultiplier
	}
	if candidates < 1 {
		candidates = 1
	}
	if candidates > 200 {
		candidates = 200
	}

	keywordResults := []memory.HybridKeywordResult{}
	if m.cfg.Query.Hybrid.Enabled && m.ftsAvailable {
		results, err := m.searchKeyword(ctx, cleaned, candidates)
		if err == nil {
			keywordResults = results
		}
	}

	vectorResults := []memory.HybridVectorResult{}
	if m.cfg.Store.Vector.Enabled {
		queryVec, err := m.provider.EmbedQuery(ctx, cleaned)
		if err != nil {
			return nil, err
		}
		hasVector := false
		for _, v := range queryVec {
			if v != 0 {
				hasVector = true
				break
			}
		}
		if hasVector {
			results, err := m.searchVector(ctx, queryVec, candidates)
			if err != nil {
				return nil, err
			}
			vectorResults = results
		}
	}

	if !m.cfg.Query.Hybrid.Enabled {
		return filterAndLimit(vectorResultsToSearch(vectorResults), minScore, maxResults), nil
	}

	merged := memory.MergeHybridResults(vectorResults, keywordResults, m.cfg.Query.Hybrid.VectorWeight, m.cfg.Query.Hybrid.TextWeight)
	return filterAndLimit(merged, minScore, maxResults), nil
}

func (m *MemorySearchManager) ReadFile(ctx context.Context, relPath string, from, lines *int) (map[string]any, error) {
	if m == nil {
		return nil, fmt.Errorf("memory search unavailable")
	}
	path, err := textfs.NormalizePath(relPath)
	if err != nil {
		return nil, fmt.Errorf("path required")
	}
	if !strings.HasSuffix(strings.ToLower(path), ".md") {
		return nil, fmt.Errorf("path required")
	}
	if !isAllowedMemoryPath(path, m.cfg.ExtraPaths) {
		return nil, fmt.Errorf("path required")
	}

	store := textfs.NewStore(m.db, m.bridgeID, m.loginID, m.agentID)
	entry, found, err := store.Read(ctx, path)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("file not found")
	}

	content := normalizeNewlines(entry.Content)
	if from == nil && lines == nil {
		return map[string]any{"path": entry.Path, "text": content}, nil
	}
	lineList := strings.Split(content, "\n")
	start := 1
	if from != nil && *from > 1 {
		start = *from
	}
	count := len(lineList)
	if lines != nil && *lines > 0 {
		count = *lines
	}
	if start < 1 {
		start = 1
	}
	end := start - 1 + count
	if end > len(lineList) {
		end = len(lineList)
	}
	if start > len(lineList) {
		start = len(lineList)
	}
	if start < 1 {
		start = 1
	}
	slice := lineList[start-1 : end]
	return map[string]any{"path": entry.Path, "text": strings.Join(slice, "\n")}, nil
}

func filterAndLimit(results []memory.SearchResult, minScore float64, maxResults int) []memory.SearchResult {
	filtered := results[:0]
	for _, result := range results {
		if result.Score >= minScore {
			filtered = append(filtered, result)
		}
	}
	if len(filtered) > maxResults {
		return filtered[:maxResults]
	}
	return filtered
}

func vectorResultsToSearch(results []memory.HybridVectorResult) []memory.SearchResult {
	out := make([]memory.SearchResult, 0, len(results))
	for _, entry := range results {
		out = append(out, memory.SearchResult{
			Path:      entry.Path,
			StartLine: entry.StartLine,
			EndLine:   entry.EndLine,
			Score:     entry.VectorScore,
			Snippet:   entry.Snippet,
			Source:    entry.Source,
		})
	}
	return out
}

func memoryManagerCacheKey(bridgeID, loginID, agentID string, cfg *memory.ResolvedConfig) string {
	if cfg == nil {
		return fmt.Sprintf("%s:%s:%s", bridgeID, loginID, agentID)
	}
	sources := append([]string{}, cfg.Sources...)
	extra := append([]string{}, cfg.ExtraPaths...)
	sort.Strings(sources)
	sort.Strings(extra)
	payload := map[string]any{
		"sources":       sources,
		"extraPaths":    extra,
		"provider":      cfg.Provider,
		"model":         cfg.Model,
		"fallback":      cfg.Fallback,
		"remoteBase":    cfg.Remote.BaseURL,
		"remoteHeaders": sortedHeaderNames(cfg.Remote.Headers),
		"remoteBatch": map[string]any{
			"enabled":        cfg.Remote.Batch.Enabled,
			"wait":           cfg.Remote.Batch.Wait,
			"concurrency":    cfg.Remote.Batch.Concurrency,
			"poll":           cfg.Remote.Batch.PollIntervalMs,
			"timeoutMinutes": cfg.Remote.Batch.TimeoutMinutes,
		},
		"localBase":  cfg.Local.BaseURL,
		"localModel": cfg.Local.ModelPath,
		"localKey":   hashString(cfg.Local.APIKey),
		"remoteKey":  hashString(cfg.Remote.APIKey),
		"store": map[string]any{
			"driver":        cfg.Store.Driver,
			"path":          cfg.Store.Path,
			"vectorEnabled": cfg.Store.Vector.Enabled,
			"vectorExt":     cfg.Store.Vector.ExtensionPath,
		},
		"chunking":     cfg.Chunking,
		"sync":         cfg.Sync,
		"query":        cfg.Query,
		"cache":        cfg.Cache,
		"experimental": cfg.Experimental,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%s:%s:%s:%s", bridgeID, loginID, agentID, hex.EncodeToString(sum[:]))
}

func sortedHeaderNames(headers map[string]string) []string {
	if len(headers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		trimmed := strings.ToLower(strings.TrimSpace(key))
		if trimmed == "" {
			continue
		}
		keys = append(keys, trimmed)
	}
	sort.Strings(keys)
	return keys
}

func hashString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])
}

func normalizeNewlines(text string) string {
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func truncateSnippet(text string) string {
	if text == "" {
		return ""
	}
	if len([]rune(text)) <= memorySnippetMaxChars {
		return text
	}
	runes := []rune(text)
	return string(runes[:memorySnippetMaxChars])
}

func isAllowedMemoryPath(path string, extraPaths []string) bool {
	if textfs.IsMemoryPath(path) {
		return true
	}
	if len(extraPaths) == 0 {
		return false
	}
	normalizedExtra := normalizeExtraPaths(extraPaths)
	for _, extra := range normalizedExtra {
		if strings.HasSuffix(strings.ToLower(extra), ".md") {
			if strings.EqualFold(path, extra) {
				return true
			}
			continue
		}
		if path == extra || strings.HasPrefix(path, extra+"/") {
			return true
		}
	}
	return false
}

func normalizeExtraPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		normalized, err := textfs.NormalizePath(trimmed)
		if err != nil {
			continue
		}
		normalized = strings.TrimSuffix(normalized, "/")
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}
