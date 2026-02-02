package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Memory configuration defaults (matching OpenClaw)
const (
	DefaultMemoryMaxResults = 6
	DefaultMemoryMinScore   = 0.35
	DefaultMemoryImportance = 0.5
	MaxIndexEntriesPerChunk = 100
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
	// Get agent data room
	agentDataPortal, err := m.getAgentDataRoom(ctx, agentID)
	if err != nil || agentDataPortal == nil {
		return nil, err
	}

	entries, err := m.loadMemoryIndex(ctx, agentDataPortal)
	if err != nil {
		return nil, err
	}

	return m.searchByKeywords(entries, query, minScore, MemoryScopeAgent, agentID), nil
}

// searchGlobalMemory searches the global memory room
func (m *MemoryStore) searchGlobalMemory(ctx context.Context, query string, minScore float64) ([]MemorySearchResult, error) {
	globalPortal, err := m.getGlobalMemoryRoom(ctx)
	if err != nil || globalPortal == nil {
		return nil, err
	}

	entries, err := m.loadMemoryIndex(ctx, globalPortal)
	if err != nil {
		return nil, err
	}

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
	scope, factID, agentID, ok := parseMemoryPath(input.Path)
	if !ok {
		return nil, fmt.Errorf("invalid memory path: %s", input.Path)
	}

	var memoryPortal *bridgev2.Portal
	var err error

	switch scope {
	case MemoryScopeGlobal:
		memoryPortal, err = m.getGlobalMemoryRoom(ctx)
	case MemoryScopeAgent:
		if agentID == "" {
			meta := portalMeta(portal)
			if meta != nil {
				agentID = meta.AgentID
			}
		}
		memoryPortal, err = m.getAgentDataRoom(ctx, agentID)
	default:
		return nil, fmt.Errorf("unknown memory scope: %s", scope)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get memory room: %w", err)
	}
	if memoryPortal == nil {
		return nil, fmt.Errorf("memory room not found")
	}

	// Look up the fact in the index to get the event ID
	entries, err := m.loadMemoryIndex(ctx, memoryPortal)
	if err != nil {
		return nil, fmt.Errorf("failed to load memory index: %w", err)
	}

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

	// Fetch the actual memory event
	content, err := m.fetchMemoryEvent(ctx, memoryPortal, id.EventID(targetEntry.EventID))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch memory event: %w", err)
	}

	return &MemoryGetResult{
		Text: content,
		Path: input.Path,
	}, nil
}

// Store creates a new memory
func (m *MemoryStore) Store(ctx context.Context, portal *bridgev2.Portal, input MemoryStoreInput) (*MemoryStoreResult, error) {
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

	// Get or create memory room
	var memoryPortal *bridgev2.Portal
	var agentID string
	var err error

	switch scope {
	case MemoryScopeGlobal:
		memoryPortal, err = m.getOrCreateGlobalMemoryRoom(ctx)
	case MemoryScopeAgent:
		meta := portalMeta(portal)
		if meta != nil {
			agentID = meta.AgentID
		}
		memoryPortal, err = m.getOrCreateAgentDataRoom(ctx, agentID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get memory room: %w", err)
	}
	if memoryPortal == nil {
		return nil, fmt.Errorf("memory room not available")
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
		SourceRoom: string(portal.MXID),
		CreatedAt:  now,
	}

	// Send the memory event
	eventID, err := m.sendMemoryEvent(ctx, memoryPortal, factContent)
	if err != nil {
		return nil, fmt.Errorf("failed to store memory: %w", err)
	}

	// Update the index
	preview := input.Content
	if len(preview) > MemoryPreviewLength {
		preview = preview[:MemoryPreviewLength]
	}

	indexEntry := MemoryIndexEntry{
		FactID:     factID,
		EventID:    string(eventID),
		Keywords:   keywords,
		Category:   category,
		Importance: importance,
		Preview:    preview,
		CreatedAt:  now,
	}

	if err := m.updateMemoryIndex(ctx, memoryPortal, indexEntry, false); err != nil {
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
	scope, factID, agentID, ok := parseMemoryPath(input.ID)
	if !ok {
		return nil, fmt.Errorf("invalid memory path: %s", input.ID)
	}

	var memoryPortal *bridgev2.Portal
	var err error

	switch scope {
	case MemoryScopeGlobal:
		memoryPortal, err = m.getGlobalMemoryRoom(ctx)
	case MemoryScopeAgent:
		if agentID == "" {
			meta := portalMeta(portal)
			if meta != nil {
				agentID = meta.AgentID
			}
		}
		memoryPortal, err = m.getAgentDataRoom(ctx, agentID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get memory room: %w", err)
	}
	if memoryPortal == nil {
		return &MemoryForgetResult{
			Success: false,
			Message: "memory room not found",
		}, nil
	}

	// Find the entry in the index
	entries, err := m.loadMemoryIndex(ctx, memoryPortal)
	if err != nil {
		return nil, fmt.Errorf("failed to load memory index: %w", err)
	}

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
	if err := m.updateMemoryIndex(ctx, memoryPortal, *targetEntry, true); err != nil {
		return nil, fmt.Errorf("failed to update memory index: %w", err)
	}

	return &MemoryForgetResult{
		Success: true,
		Message: "memory removed from index",
	}, nil
}

// Room management helpers

func (m *MemoryStore) getAgentDataRoom(ctx context.Context, agentID string) (*bridgev2.Portal, error) {
	if agentID == "" {
		return nil, nil
	}

	loginID := m.client.UserLogin.ID
	portalKey := agentDataPortalKey(loginID, agentID)

	portal, err := m.client.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
	if err != nil {
		return nil, err
	}

	return portal, nil
}

func (m *MemoryStore) getOrCreateAgentDataRoom(ctx context.Context, agentID string) (*bridgev2.Portal, error) {
	if agentID == "" {
		return nil, nil
	}

	// First try to get existing room
	portal, err := m.getAgentDataRoom(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if portal != nil && portal.MXID != "" {
		return portal, nil
	}

	// Create agent memory room on demand
	loginID := m.client.UserLogin.ID
	portalKey := agentDataPortalKey(loginID, agentID)

	portal, err = m.client.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get portal: %w", err)
	}

	if portal.MXID == "" {
		// Need to create the Matrix room
		roomName := fmt.Sprintf("Agent Memory: %s", agentID)
		chatInfo := &bridgev2.ChatInfo{
			Name:  &roomName,
			Topic: strPtr(fmt.Sprintf("Memory storage for agent %s", agentID)),
		}

		err = portal.CreateMatrixRoom(ctx, m.client.UserLogin, chatInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent memory room: %w", err)
		}

		// Set metadata
		meta := portalMeta(portal)
		meta.IsAgentDataRoom = true
		meta.AgentID = agentID
		if err := portal.Save(ctx); err != nil {
			m.client.log.Warn().Err(err).Str("agent_id", agentID).Msg("Failed to save agent memory room metadata")
		}

		m.client.log.Info().Str("agent_id", agentID).Msg("Created agent memory room on demand")
	}

	return portal, nil
}

func (m *MemoryStore) getGlobalMemoryRoom(ctx context.Context) (*bridgev2.Portal, error) {
	loginMeta := loginMetadata(m.client.UserLogin)
	if loginMeta.GlobalMemoryRoomID == "" {
		return nil, nil
	}

	portalKey := networkid.PortalKey{
		ID:       loginMeta.GlobalMemoryRoomID,
		Receiver: m.client.UserLogin.ID,
	}

	portal, err := m.client.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
	if err != nil {
		return nil, err
	}

	return portal, nil
}

func (m *MemoryStore) getOrCreateGlobalMemoryRoom(ctx context.Context) (*bridgev2.Portal, error) {
	// First try to get existing room
	portal, err := m.getGlobalMemoryRoom(ctx)
	if err != nil {
		return nil, err
	}
	if portal != nil && portal.MXID != "" {
		return portal, nil
	}

	// Create new global memory room
	loginID := m.client.UserLogin.ID
	portalKey := globalMemoryPortalKey(loginID)

	portal, err = m.client.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
	if err != nil {
		return nil, err
	}

	if portal.MXID == "" {
		// Need to create the Matrix room
		chatInfo := &bridgev2.ChatInfo{
			Name:  strPtr("Global Memory"),
			Topic: strPtr("Shared memory storage for all agents"),
		}

		err = portal.CreateMatrixRoom(ctx, m.client.UserLogin, chatInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to create global memory room: %w", err)
		}

		// Set metadata
		meta := portalMeta(portal)
		meta.IsGlobalMemoryRoom = true
		if err := portal.Save(ctx); err != nil {
			m.client.log.Warn().Err(err).Msg("Failed to save global memory room metadata")
		}

		// Update login metadata with room ID
		loginMeta := loginMetadata(m.client.UserLogin)
		loginMeta.GlobalMemoryRoomID = portalKey.ID
		if err := m.client.UserLogin.Save(ctx); err != nil {
			m.client.log.Warn().Err(err).Msg("Failed to save login metadata with global memory room ID")
		}
	}

	return portal, nil
}

// Index management helpers

func (m *MemoryStore) loadMemoryIndex(ctx context.Context, portal *bridgev2.Portal) ([]MemoryIndexEntry, error) {
	if portal == nil || portal.MXID == "" {
		return nil, nil
	}

	matrixConn, ok := m.client.UserLogin.Bridge.Matrix.(*matrix.Connector)
	if !ok {
		return nil, fmt.Errorf("matrix connector not available")
	}

	var allEntries []MemoryIndexEntry

	// Load all index chunks (state key format: "0", "1", "2", etc.)
	for chunkID := 0; ; chunkID++ {
		stateKey := fmt.Sprintf("%d", chunkID)
		evt, err := matrixConn.GetStateEvent(ctx, portal.MXID, MemoryIndexEventType, stateKey)
		if err != nil || evt == nil {
			break
		}

		var indexContent MemoryIndexContent
		if err := json.Unmarshal(evt.Content.VeryRaw, &indexContent); err != nil {
			m.client.log.Warn().Err(err).Int("chunk", chunkID).Msg("Failed to parse memory index chunk")
			continue
		}

		allEntries = append(allEntries, indexContent.Entries...)

		if chunkID >= indexContent.TotalChunks-1 {
			break
		}
	}

	return allEntries, nil
}

func (m *MemoryStore) updateMemoryIndex(ctx context.Context, portal *bridgev2.Portal, entry MemoryIndexEntry, remove bool) error {
	if portal == nil || portal.MXID == "" {
		return fmt.Errorf("portal not available")
	}

	// Load existing index
	entries, err := m.loadMemoryIndex(ctx, portal)
	if err != nil {
		return err
	}

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

	// Save the index (chunked if necessary)
	return m.saveMemoryIndex(ctx, portal, entries)
}

func (m *MemoryStore) saveMemoryIndex(ctx context.Context, portal *bridgev2.Portal, entries []MemoryIndexEntry) error {
	bot := m.client.UserLogin.Bridge.Bot

	totalChunks := (len(entries) + MaxIndexEntriesPerChunk - 1) / MaxIndexEntriesPerChunk
	if totalChunks == 0 {
		totalChunks = 1
	}

	now := time.Now().UnixMilli()

	for chunkID := 0; chunkID < totalChunks; chunkID++ {
		start := chunkID * MaxIndexEntriesPerChunk
		end := start + MaxIndexEntriesPerChunk
		if end > len(entries) {
			end = len(entries)
		}

		chunkEntries := entries[start:end]
		if start >= len(entries) {
			chunkEntries = nil
		}

		indexContent := MemoryIndexContent{
			ChunkID:     chunkID,
			TotalChunks: totalChunks,
			Entries:     chunkEntries,
			UpdatedAt:   now,
		}

		stateKey := fmt.Sprintf("%d", chunkID)
		_, err := bot.SendState(ctx, portal.MXID, MemoryIndexEventType, stateKey, &event.Content{
			Parsed: &indexContent,
		}, time.Time{})
		if err != nil {
			return fmt.Errorf("failed to save memory index chunk %d: %w", chunkID, err)
		}
	}

	return nil
}

// Event helpers

func (m *MemoryStore) sendMemoryEvent(ctx context.Context, portal *bridgev2.Portal, content *MemoryFactContent) (id.EventID, error) {
	bot := m.client.UserLogin.Bridge.Bot

	resp, err := bot.SendMessage(ctx, portal.MXID, MemoryFactEventType, &event.Content{
		Parsed: content,
	}, nil)
	if err != nil {
		return "", err
	}

	return resp.EventID, nil
}

func (m *MemoryStore) fetchMemoryEvent(ctx context.Context, portal *bridgev2.Portal, eventID id.EventID) (string, error) {
	matrixConn, ok := m.client.UserLogin.Bridge.Matrix.(*matrix.Connector)
	if !ok {
		return "", fmt.Errorf("matrix connector not available")
	}

	evt, err := matrixConn.AS.BotClient().GetEvent(ctx, portal.MXID, eventID)
	if err != nil {
		return "", err
	}

	var factContent MemoryFactContent
	if err := json.Unmarshal(evt.Content.VeryRaw, &factContent); err != nil {
		return "", fmt.Errorf("failed to parse memory event: %w", err)
	}

	return factContent.Content, nil
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

// strPtr is a helper to create pointers to strings
func strPtr(v string) *string {
	return &v
}
