package connector

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/ai-bridge/pkg/agents"
)

// Memory configuration defaults (matching OpenClaw)
const (
	DefaultMemoryMaxResults = 6
	DefaultMemoryMinScore   = 0.35
	DefaultMemoryImportance = 0.5
	MemoryPreviewLength     = 100
)

// Memory tool input/output types (matching OpenClaw interface)

// MemorySearchInput matches OpenClaw's memory_search input
type MemorySearchInput struct {
	Query      string   `json:"query"`
	MaxResults *int     `json:"maxResults,omitempty"` // default: 6
	MinScore   *float64 `json:"minScore,omitempty"`   // default: 0.35
}

// MemorySearchResult matches OpenClaw's memory_search output
type MemorySearchResult struct {
	Path      string  `json:"path"`      // "agent:{id}/fact:{id}" or "global/fact:{id}"
	StartLine int     `json:"startLine"` // Always 0 for Matrix
	EndLine   int     `json:"endLine"`   // Always 0 for Matrix
	Score     float64 `json:"score"`
	Snippet   string  `json:"snippet"`
	Source    string  `json:"source"` // "memory"
}

// MemoryGetInput matches OpenClaw's memory_get input
type MemoryGetInput struct {
	Path  string `json:"path"`
	From  *int   `json:"from,omitempty"`  // Ignored for Matrix
	Lines *int   `json:"lines,omitempty"` // Ignored for Matrix
}

// MemoryGetResult matches OpenClaw's memory_get output
type MemoryGetResult struct {
	Text string `json:"text"`
	Path string `json:"path"`
}

// MemoryStoreInput matches OpenClaw's memory_store input
type MemoryStoreInput struct {
	Content    string   `json:"content"`
	Importance *float64 `json:"importance,omitempty"` // 0-1, default 0.5
	Category   *string  `json:"category,omitempty"`   // preference, decision, entity, fact, other
	Scope      *string  `json:"scope,omitempty"`      // "agent" or "global", default "agent"
}

// MemoryStoreResult matches OpenClaw's memory_store output
type MemoryStoreResult struct {
	ID      string `json:"id"` // Full path
	Success bool   `json:"success"`
}

// MemoryForgetInput matches OpenClaw's memory_forget input
type MemoryForgetInput struct {
	ID string `json:"id"` // Full path or just fact ID
}

// MemoryForgetResult matches OpenClaw's memory_forget output
type MemoryForgetResult struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// MemoryStore handles memory operations for an AI client
type MemoryStore struct {
	client *AIClient
	mu     sync.Mutex // protects metadata memory read-modify-write sequences
}

// NewMemoryStore creates a new memory store for the given client
func NewMemoryStore(client *AIClient) *MemoryStore {
	return &MemoryStore{client: client}
}

// getEffectiveConfig returns the effective memory configuration for the current agent
func (m *MemoryStore) getEffectiveConfig(portal *bridgev2.Portal) *AgentMemoryConfig {
	meta := portalMeta(portal)
	if meta == nil {
		return defaultMemoryConfig()
	}

	// Get agent-specific config from agent definition
	if meta.AgentID != "" {
		store := NewAgentStoreAdapter(m.client)
		agent, err := store.GetAgentByID(context.Background(), meta.AgentID)
		if err == nil && agent != nil && agent.Memory != nil {
			// Convert agents.MemoryConfig to AgentMemoryConfig
			return &AgentMemoryConfig{
				Enabled:      agent.Memory.Enabled,
				Sources:      agent.Memory.Sources,
				EnableGlobal: agent.Memory.EnableGlobal,
				MaxResults:   agent.Memory.MaxResults,
				MinScore:     agent.Memory.MinScore,
			}
		}
	}

	return defaultMemoryConfig()
}

// defaultMemoryConfig returns the default memory configuration
func defaultMemoryConfig() *AgentMemoryConfig {
	enabled := true
	enableGlobal := true
	return &AgentMemoryConfig{
		Enabled:      &enabled,
		Sources:      []string{"memory"},
		EnableGlobal: &enableGlobal,
		MaxResults:   DefaultMemoryMaxResults,
		MinScore:     DefaultMemoryMinScore,
	}
}

// isMemoryEnabled checks if memory is enabled for the given scope
func (m *MemoryStore) isMemoryEnabled(config *AgentMemoryConfig, scope MemoryScope) bool {
	if config == nil {
		return true
	}
	if config.Enabled != nil && !*config.Enabled {
		return false
	}
	if scope == MemoryScopeGlobal && config.EnableGlobal != nil && !*config.EnableGlobal {
		return false
	}
	return true
}

// Search searches for memories matching the query
func (m *MemoryStore) Search(ctx context.Context, portal *bridgev2.Portal, input MemorySearchInput) ([]MemorySearchResult, error) {
	config := m.getEffectiveConfig(portal)

	maxResults := DefaultMemoryMaxResults
	if input.MaxResults != nil && *input.MaxResults > 0 {
		maxResults = *input.MaxResults
	} else if config.MaxResults > 0 {
		maxResults = config.MaxResults
	}

	minScore := DefaultMemoryMinScore
	if input.MinScore != nil && *input.MinScore >= 0 {
		minScore = *input.MinScore
	} else if config.MinScore > 0 {
		minScore = config.MinScore
	}

	var allResults []MemorySearchResult

	// Search agent memory if enabled
	if m.isMemoryEnabled(config, MemoryScopeAgent) {
		meta := portalMeta(portal)
		agentID := ""
		if meta != nil {
			agentID = meta.AgentID
		}
		agentResults, err := m.searchAgentMemory(ctx, input.Query, agentID, minScore)
		if err != nil {
			m.client.log.Warn().Err(err).Msg("Failed to search agent memory")
		} else {
			allResults = append(allResults, agentResults...)
		}
	}

	// Search global memory if enabled
	if m.isMemoryEnabled(config, MemoryScopeGlobal) {
		globalResults, err := m.searchGlobalMemory(ctx, input.Query, minScore)
		if err != nil {
			m.client.log.Warn().Err(err).Msg("Failed to search global memory")
		} else {
			allResults = append(allResults, globalResults...)
		}
	}

	// Sort by score descending
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	// Limit results
	if len(allResults) > maxResults {
		allResults = allResults[:maxResults]
	}

	return allResults, nil
}

// searchAgentMemory searches the agent's memory room
func (m *MemoryStore) searchAgentMemory(ctx context.Context, query string, agentID string, minScore float64) ([]MemorySearchResult, error) {
	resolvedAgentID := m.resolveAgentID(nil, agentID)
	entries := m.loadMemoryIndexByScope(MemoryScopeAgent, resolvedAgentID)
	return m.searchByKeywords(entries, query, minScore, MemoryScopeAgent, resolvedAgentID), nil
}

// searchGlobalMemory searches the global memory room
func (m *MemoryStore) searchGlobalMemory(ctx context.Context, query string, minScore float64) ([]MemorySearchResult, error) {
	_ = ctx
	entries := m.loadMemoryIndexByScope(MemoryScopeGlobal, "")
	return m.searchByKeywords(entries, query, minScore, MemoryScopeGlobal, ""), nil
}

// searchByKeywords performs keyword-based search on memory entries
func (m *MemoryStore) searchByKeywords(entries []MemoryIndexEntry, query string, minScore float64, scope MemoryScope, agentID string) []MemorySearchResult {
	queryWords := tokenize(strings.ToLower(query))
	var results []MemorySearchResult

	for _, entry := range entries {
		score := calculateScore(queryWords, entry.Keywords, entry.Importance)
		if score >= minScore {
			results = append(results, MemorySearchResult{
				Path:      formatMemoryPath(scope, entry.FactID, agentID),
				StartLine: 0,
				EndLine:   0,
				Score:     score,
				Snippet:   entry.Preview,
				Source:    "memory",
			})
		}
	}

	return results
}

// Get retrieves a specific memory by path
func (m *MemoryStore) Get(ctx context.Context, portal *bridgev2.Portal, input MemoryGetInput) (*MemoryGetResult, error) {
	_ = ctx
	scope, factID, agentID, ok := parseMemoryPath(input.Path)
	if !ok {
		return nil, fmt.Errorf("invalid memory path: %s", input.Path)
	}

	switch scope {
	case MemoryScopeGlobal:
	case MemoryScopeAgent:
		agentID = m.resolveAgentID(portal, agentID)
	default:
		return nil, fmt.Errorf("unknown memory scope: %s", scope)
	}

	// Look up the fact in the index to get the event ID
	entries := m.loadMemoryIndexByScope(scope, agentID)

	var targetEntry *MemoryIndexEntry
	for i := range entries {
		if entries[i].FactID == factID {
			targetEntry = &entries[i]
			break
		}
	}

	if targetEntry == nil {
		return nil, fmt.Errorf("memory not found: %s", factID)
	}

	fact := m.loadMemoryFactFromMetadata(scope, agentID, factID)
	if fact == nil {
		return nil, fmt.Errorf("memory content not found: %s", factID)
	}

	return &MemoryGetResult{
		Text: fact.Content,
		Path: input.Path,
	}, nil
}

// Store creates a new memory
func (m *MemoryStore) Store(ctx context.Context, portal *bridgev2.Portal, input MemoryStoreInput) (*MemoryStoreResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	config := m.getEffectiveConfig(portal)

	// Determine scope
	scope := MemoryScopeAgent
	if input.Scope != nil && *input.Scope == "global" {
		scope = MemoryScopeGlobal
	}

	// Check if memory is enabled for this scope
	if !m.isMemoryEnabled(config, scope) {
		return &MemoryStoreResult{
			ID:      "",
			Success: false,
		}, fmt.Errorf("memory is disabled for scope: %s", scope)
	}

	var agentID string

	switch scope {
	case MemoryScopeGlobal:
	case MemoryScopeAgent:
		agentID = m.resolveAgentID(portal, "")
	}

	// Generate fact ID
	factID := generateShortID()

	// Extract keywords from content
	keywords := extractKeywords(input.Content)

	// Set importance
	importance := DefaultMemoryImportance
	if input.Importance != nil {
		importance = *input.Importance
		if importance < 0 {
			importance = 0
		} else if importance > 1 {
			importance = 1
		}
	}

	// Set category
	category := "other"
	if input.Category != nil && *input.Category != "" {
		category = *input.Category
	}

	// Create memory fact content
	now := time.Now().UnixMilli()
	factContent := &MemoryFactContent{
		FactID:     factID,
		Content:    input.Content,
		Keywords:   keywords,
		Category:   category,
		Importance: importance,
		Source:     "assistant",
		SourceRoom: "",
		CreatedAt:  now,
	}
	if portal != nil {
		factContent.SourceRoom = string(portal.MXID)
	}
	if err := m.saveMemoryFactToMetadata(ctx, scope, agentID, factContent); err != nil {
		return nil, fmt.Errorf("failed to store memory fact: %w", err)
	}

	// Update the index
	preview := input.Content
	if len(preview) > MemoryPreviewLength {
		preview = preview[:MemoryPreviewLength]
	}

	indexEntry := MemoryIndexEntry{
		FactID:     factID,
		EventID:    "",
		Keywords:   keywords,
		Category:   category,
		Importance: importance,
		Preview:    preview,
		CreatedAt:  now,
	}

	if err := m.updateMemoryIndex(ctx, scope, agentID, indexEntry, false); err != nil {
		m.client.log.Warn().Err(err).Msg("Failed to update memory index")
	}

	path := formatMemoryPath(scope, factID, agentID)

	return &MemoryStoreResult{
		ID:      path,
		Success: true,
	}, nil
}

// Forget removes a memory
func (m *MemoryStore) Forget(ctx context.Context, portal *bridgev2.Portal, input MemoryForgetInput) (*MemoryForgetResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	scope, factID, agentID, ok := parseMemoryPath(input.ID)
	if !ok {
		return nil, fmt.Errorf("invalid memory path: %s", input.ID)
	}

	switch scope {
	case MemoryScopeGlobal:
	case MemoryScopeAgent:
		agentID = m.resolveAgentID(portal, agentID)
	}

	// Find the entry in the index
	entries := m.loadMemoryIndexByScope(scope, agentID)

	var targetEntry *MemoryIndexEntry
	for i := range entries {
		if entries[i].FactID == factID {
			targetEntry = &entries[i]
			break
		}
	}

	if targetEntry == nil {
		return &MemoryForgetResult{
			Success: false,
			Message: "memory not found",
		}, nil
	}

	// Remove from index (create an entry with empty EventID to mark as removed)
	if err := m.updateMemoryIndex(ctx, scope, agentID, *targetEntry, true); err != nil {
		return nil, fmt.Errorf("failed to update memory index: %w", err)
	}
	if err := m.deleteMemoryFactFromMetadata(ctx, scope, agentID, factID); err != nil {
		m.client.log.Warn().Err(err).Str("fact_id", factID).Msg("Failed to delete memory fact from metadata")
	}

	return &MemoryForgetResult{
		Success: true,
		Message: "memory removed from index",
	}, nil
}

// Index management helpers

func (m *MemoryStore) memoryScopeKey(scope MemoryScope, agentID string) string {
	if scope == MemoryScopeGlobal {
		return "global"
	}
	return "agent:" + m.resolveAgentID(nil, agentID)
}

func (m *MemoryStore) resolveAgentID(portal *bridgev2.Portal, agentID string) string {
	if strings.TrimSpace(agentID) != "" {
		return strings.TrimSpace(agentID)
	}
	if portal != nil {
		meta := portalMeta(portal)
		if meta != nil && strings.TrimSpace(meta.AgentID) != "" {
			return strings.TrimSpace(meta.AgentID)
		}
	}
	return agents.DefaultAgentID
}

func (m *MemoryStore) loadMemoryIndexByScope(scope MemoryScope, agentID string) []MemoryIndexEntry {
	meta := loginMetadata(m.client.UserLogin)
	if meta == nil || len(meta.MemoryIndexes) == 0 {
		return nil
	}
	key := m.memoryScopeKey(scope, agentID)
	entries := meta.MemoryIndexes[key]
	if len(entries) == 0 {
		return nil
	}
	copied := make([]MemoryIndexEntry, len(entries))
	copy(copied, entries)
	return copied
}

func (m *MemoryStore) saveMemoryIndexByScope(ctx context.Context, scope MemoryScope, agentID string, entries []MemoryIndexEntry) error {
	meta := loginMetadata(m.client.UserLogin)
	if meta.MemoryIndexes == nil {
		meta.MemoryIndexes = make(map[string][]MemoryIndexEntry)
	}
	key := m.memoryScopeKey(scope, agentID)
	copied := make([]MemoryIndexEntry, len(entries))
	copy(copied, entries)
	meta.MemoryIndexes[key] = copied
	return m.client.UserLogin.Save(ctx)
}

func (m *MemoryStore) updateMemoryIndex(ctx context.Context, scope MemoryScope, agentID string, entry MemoryIndexEntry, remove bool) error {
	// Load existing index
	entries := m.loadMemoryIndexByScope(scope, agentID)

	if remove {
		// Remove the entry
		var newEntries []MemoryIndexEntry
		for _, e := range entries {
			if e.FactID != entry.FactID {
				newEntries = append(newEntries, e)
			}
		}
		entries = newEntries
	} else {
		// Add or update the entry
		found := false
		for i := range entries {
			if entries[i].FactID == entry.FactID {
				entries[i] = entry
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, entry)
		}
	}

	return m.saveMemoryIndexByScope(ctx, scope, agentID, entries)
}

func (m *MemoryStore) loadMemoryFactFromMetadata(scope MemoryScope, agentID, factID string) *MemoryFactContent {
	meta := loginMetadata(m.client.UserLogin)
	if meta == nil || len(meta.MemoryFacts) == 0 {
		return nil
	}
	scopeKey := m.memoryScopeKey(scope, agentID)
	facts := meta.MemoryFacts[scopeKey]
	if len(facts) == 0 {
		return nil
	}
	return facts[factID]
}

func (m *MemoryStore) saveMemoryFactToMetadata(ctx context.Context, scope MemoryScope, agentID string, fact *MemoryFactContent) error {
	if fact == nil || fact.FactID == "" {
		return fmt.Errorf("memory fact is required")
	}
	meta := loginMetadata(m.client.UserLogin)
	if meta.MemoryFacts == nil {
		meta.MemoryFacts = make(map[string]map[string]*MemoryFactContent)
	}
	scopeKey := m.memoryScopeKey(scope, agentID)
	if meta.MemoryFacts[scopeKey] == nil {
		meta.MemoryFacts[scopeKey] = make(map[string]*MemoryFactContent)
	}
	meta.MemoryFacts[scopeKey][fact.FactID] = fact
	return m.client.UserLogin.Save(ctx)
}

func (m *MemoryStore) deleteMemoryFactFromMetadata(ctx context.Context, scope MemoryScope, agentID, factID string) error {
	meta := loginMetadata(m.client.UserLogin)
	if meta == nil || meta.MemoryFacts == nil {
		return nil
	}
	scopeKey := m.memoryScopeKey(scope, agentID)
	facts := meta.MemoryFacts[scopeKey]
	if facts == nil {
		return nil
	}
	if _, ok := facts[factID]; !ok {
		return nil
	}
	delete(facts, factID)
	return m.client.UserLogin.Save(ctx)
}

// Keyword extraction and search helpers

// tokenize splits text into lowercase words for search
func tokenize(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			current.WriteRune(unicode.ToLower(r))
		} else if current.Len() > 0 {
			word := current.String()
			if len(word) >= 2 { // Skip single-char words
				words = append(words, word)
			}
			current.Reset()
		}
	}

	if current.Len() > 0 {
		word := current.String()
		if len(word) >= 2 {
			words = append(words, word)
		}
	}

	return words
}

// extractKeywords extracts important keywords from content
func extractKeywords(content string) []string {
	words := tokenize(strings.ToLower(content))

	// Remove common stop words
	stopWords := map[string]bool{
		"the": true, "be": true, "to": true, "of": true, "and": true,
		"in": true, "that": true, "have": true, "it": true, "for": true,
		"not": true, "on": true, "with": true, "he": true, "as": true,
		"you": true, "do": true, "at": true, "this": true, "but": true,
		"his": true, "by": true, "from": true, "they": true, "we": true,
		"say": true, "her": true, "she": true, "or": true, "an": true,
		"will": true, "my": true, "one": true, "all": true, "would": true,
		"there": true, "their": true, "what": true, "so": true, "up": true,
		"out": true, "if": true, "about": true, "who": true, "get": true,
		"which": true, "go": true, "me": true, "is": true, "are": true,
		"was": true, "were": true, "been": true, "being": true, "has": true,
		"had": true, "does": true, "did": true, "can": true, "could": true,
		"should": true, "may": true, "might": true, "must": true,
	}

	// Count word frequency
	wordCount := make(map[string]int)
	for _, word := range words {
		if !stopWords[word] && len(word) >= 3 {
			wordCount[word]++
		}
	}

	// Get top keywords
	type wordFreq struct {
		word  string
		count int
	}
	var freqs []wordFreq
	for word, count := range wordCount {
		freqs = append(freqs, wordFreq{word, count})
	}
	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i].count > freqs[j].count
	})

	// Return top 10 keywords
	var keywords []string
	for i, wf := range freqs {
		if i >= 10 {
			break
		}
		keywords = append(keywords, wf.word)
	}

	return keywords
}

// calculateScore computes a relevance score for a memory entry
func calculateScore(queryWords, keywords []string, importance float64) float64 {
	if len(queryWords) == 0 {
		return 0
	}

	matches := 0
	for _, qWord := range queryWords {
		for _, kWord := range keywords {
			if strings.Contains(strings.ToLower(kWord), qWord) {
				matches++
				break
			}
		}
	}

	// Base score from keyword matches, boosted by importance
	baseScore := float64(matches) / float64(len(queryWords))
	return baseScore * (0.5 + importance*0.5) // importance affects score by up to 50%
}
