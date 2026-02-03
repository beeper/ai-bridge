package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/memory"
	"github.com/beeper/ai-bridge/pkg/textfs"
	"github.com/google/uuid"
)

func (m *MemorySearchManager) ensureSchema(ctx context.Context) {
	if m == nil || m.db == nil {
		return
	}
	if !m.cfg.Query.Hybrid.Enabled {
		m.ftsAvailable = false
		return
	}
	_, err := m.db.Exec(ctx,
		`CREATE VIRTUAL TABLE IF NOT EXISTS ai_memory_chunks_fts USING fts5(
			text,
			id UNINDEXED,
			path UNINDEXED,
			source UNINDEXED,
			model UNINDEXED,
			start_line UNINDEXED,
			end_line UNINDEXED,
			bridge_id UNINDEXED,
			login_id UNINDEXED,
			agent_id UNINDEXED
		);`,
	)
	if err != nil {
		m.ftsAvailable = false
		m.ftsError = err.Error()
		return
	}
	m.ftsAvailable = true
}

func (m *MemorySearchManager) sync(ctx context.Context, sessionKey string, force bool) error {
	if m == nil {
		return fmt.Errorf("memory search unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	needsFullReindex, err := m.needsFullReindex(ctx, force)
	if err != nil {
		return err
	}
	if needsFullReindex {
		if err := m.clearIndex(ctx); err != nil {
			return err
		}
	}

	if err := m.indexMemoryFiles(ctx, needsFullReindex); err != nil {
		return err
	}

	if m.cfg.Experimental.SessionMemory && hasSource(m.cfg.Sources, "sessions") {
		if err := m.syncSessions(ctx, needsFullReindex, sessionKey); err != nil {
			return err
		}
	}

	return m.updateMeta(ctx)
}

func (m *MemorySearchManager) needsFullReindex(ctx context.Context, force bool) (bool, error) {
	if force {
		return true, nil
	}
	var provider, model, providerKey string
	var chunkTokens, chunkOverlap int
	var vectorDims sql.NullInt64
	row := m.db.QueryRow(ctx,
		`SELECT provider, model, provider_key, chunk_tokens, chunk_overlap, vector_dims
         FROM ai_memory_meta
         WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3`,
		m.bridgeID, m.loginID, m.agentID,
	)
	switch err := row.Scan(&provider, &model, &providerKey, &chunkTokens, &chunkOverlap, &vectorDims); err {
	case nil:
		if provider != m.status.Provider ||
			model != m.status.Model ||
			providerKey != m.providerKey ||
			chunkTokens != m.cfg.Chunking.Tokens ||
			chunkOverlap != m.cfg.Chunking.Overlap {
			return true, nil
		}
		if vectorDims.Valid {
			m.vectorDims = int(vectorDims.Int64)
		}
		return false, nil
	case sql.ErrNoRows:
		return true, nil
	default:
		return false, err
	}
}

func (m *MemorySearchManager) clearIndex(ctx context.Context) error {
	_, err := m.db.Exec(ctx,
		`DELETE FROM ai_memory_chunks WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3`,
		m.bridgeID, m.loginID, m.agentID,
	)
	if err != nil {
		return err
	}
	if m.ftsAvailable {
		_, _ = m.db.Exec(ctx,
			`DELETE FROM ai_memory_chunks_fts WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3`,
			m.bridgeID, m.loginID, m.agentID,
		)
	}
	return nil
}

func (m *MemorySearchManager) updateMeta(ctx context.Context) error {
	vectorDims := m.vectorDims
	_, err := m.db.Exec(ctx,
		`INSERT INTO ai_memory_meta
           (bridge_id, login_id, agent_id, provider, model, provider_key, chunk_tokens, chunk_overlap, vector_dims, updated_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
         ON CONFLICT (bridge_id, login_id, agent_id)
         DO UPDATE SET provider=excluded.provider, model=excluded.model, provider_key=excluded.provider_key,
           chunk_tokens=excluded.chunk_tokens, chunk_overlap=excluded.chunk_overlap, vector_dims=excluded.vector_dims, updated_at=excluded.updated_at`,
		m.bridgeID, m.loginID, m.agentID,
		m.status.Provider, m.status.Model, m.providerKey,
		m.cfg.Chunking.Tokens, m.cfg.Chunking.Overlap,
		vectorDimsOrNull(vectorDims),
		time.Now().UnixMilli(),
	)
	return err
}

func vectorDimsOrNull(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func (m *MemorySearchManager) indexMemoryFiles(ctx context.Context, force bool) error {
	store := textfs.NewStore(m.db, m.bridgeID, m.loginID, m.agentID)
	entries, err := store.List(ctx)
	if err != nil {
		return err
	}
	extraPaths := normalizeExtraPaths(m.cfg.ExtraPaths)
	activePaths := make(map[string]textfs.FileEntry)

	for _, entry := range entries {
		path := strings.TrimSpace(entry.Path)
		if path == "" {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			continue
		}
		if !textfs.IsMemoryPath(path) && !isExtraPath(path, extraPaths) {
			continue
		}
		activePaths[path] = entry
	}

	for _, entry := range activePaths {
		source := "memory"
		needs, err := m.needsFileIndex(ctx, entry, source)
		if err != nil {
			return err
		}
		if !force && !needs {
			continue
		}
		if err := m.indexContent(ctx, entry.Path, source, entry.Content); err != nil {
			return err
		}
	}

	if err := m.removeStaleMemoryChunks(ctx, activePaths); err != nil {
		return err
	}
	return nil
}

func (m *MemorySearchManager) needsFileIndex(ctx context.Context, entry textfs.FileEntry, source string) (bool, error) {
	var updatedAt sql.NullInt64
	row := m.db.QueryRow(ctx,
		`SELECT MAX(updated_at) FROM ai_memory_chunks
         WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3 AND path=$4 AND source=$5 AND model=$6`,
		m.bridgeID, m.loginID, m.agentID, entry.Path, source, m.status.Model,
	)
	if err := row.Scan(&updatedAt); err != nil {
		return true, err
	}
	if !updatedAt.Valid {
		return true, nil
	}
	if entry.UpdatedAt > updatedAt.Int64 {
		return true, nil
	}
	return false, nil
}

func (m *MemorySearchManager) indexContent(ctx context.Context, path, source, content string) error {
	cleanContent := normalizeNewlines(content)
	chunks := memory.ChunkMarkdown(cleanContent, m.cfg.Chunking.Tokens, m.cfg.Chunking.Overlap)
	if len(chunks) == 0 {
		return nil
	}

	if _, err := m.db.Exec(ctx,
		`DELETE FROM ai_memory_chunks
         WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3 AND path=$4 AND source=$5 AND model=$6`,
		m.bridgeID, m.loginID, m.agentID, path, source, m.status.Model,
	); err != nil {
		return err
	}
	if m.ftsAvailable {
		_, _ = m.db.Exec(ctx,
			`DELETE FROM ai_memory_chunks_fts
             WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3 AND path=$4 AND source=$5 AND model=$6`,
			m.bridgeID, m.loginID, m.agentID, path, source, m.status.Model,
		)
	}

	embeddings, err := m.embedChunks(ctx, chunks)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for i, chunk := range chunks {
		embedding := []float64{}
		if i < len(embeddings) {
			embedding = embeddings[i]
		}
		if m.vectorDims == 0 && len(embedding) > 0 {
			m.vectorDims = len(embedding)
		}
		embeddingJSON, _ := json.Marshal(embedding)
		chunkID := uuid.NewString()
		_, err := m.db.Exec(ctx,
			`INSERT INTO ai_memory_chunks
             (id, bridge_id, login_id, agent_id, path, source, start_line, end_line, hash, model, text, embedding, updated_at)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			chunkID, m.bridgeID, m.loginID, m.agentID, path, source, chunk.StartLine, chunk.EndLine, chunk.Hash,
			m.status.Model, chunk.Text, string(embeddingJSON), now,
		)
		if err != nil {
			return err
		}
		if m.ftsAvailable {
			_, _ = m.db.Exec(ctx,
				`INSERT INTO ai_memory_chunks_fts
                 (text, id, path, source, model, start_line, end_line, bridge_id, login_id, agent_id)
                 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				chunk.Text, chunkID, path, source, m.status.Model, chunk.StartLine, chunk.EndLine, m.bridgeID, m.loginID, m.agentID,
			)
		}
	}

	return nil
}

func (m *MemorySearchManager) removeStaleMemoryChunks(ctx context.Context, active map[string]textfs.FileEntry) error {
	rows, err := m.db.Query(ctx,
		`SELECT DISTINCT path FROM ai_memory_chunks
         WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3 AND source=$4`,
		m.bridgeID, m.loginID, m.agentID, "memory",
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var stalePaths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return err
		}
		if _, ok := active[path]; !ok {
			stalePaths = append(stalePaths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, path := range stalePaths {
		_, _ = m.db.Exec(ctx,
			`DELETE FROM ai_memory_chunks
             WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3 AND path=$4 AND source=$5`,
			m.bridgeID, m.loginID, m.agentID, path, "memory",
		)
		if m.ftsAvailable {
			_, _ = m.db.Exec(ctx,
				`DELETE FROM ai_memory_chunks_fts
                 WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3 AND path=$4 AND source=$5`,
				m.bridgeID, m.loginID, m.agentID, path, "memory",
			)
		}
	}
	return nil
}

func (m *MemorySearchManager) embedChunks(ctx context.Context, chunks []memory.Chunk) ([][]float64, error) {
	embeddings := make([][]float64, len(chunks))
	var missing []int
	for i, chunk := range chunks {
		if m.cfg.Cache.Enabled {
			if cached, ok := m.lookupEmbeddingCache(ctx, chunk.Hash); ok {
				embeddings[i] = cached
				continue
			}
		}
		missing = append(missing, i)
	}
	if len(missing) == 0 {
		return embeddings, nil
	}

	texts := make([]string, len(missing))
	for i, idx := range missing {
		texts[i] = chunks[idx].Text
	}

	results, err := m.embedBatchWithRetry(ctx, texts)
	if err != nil {
		return nil, err
	}
	for i, idx := range missing {
		if i < len(results) {
			embeddings[idx] = results[i]
			if m.cfg.Cache.Enabled {
				_ = m.storeEmbeddingCache(ctx, chunks[idx].Hash, results[i])
			}
		}
	}
	return embeddings, nil
}

func (m *MemorySearchManager) embedBatchWithRetry(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	const maxBatchSize = 64
	var results [][]float64
	for start := 0; start < len(texts); start += maxBatchSize {
		end := start + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]
		var batchResult [][]float64
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			batchResult, err = m.provider.EmbedBatch(ctx, batch)
			if err == nil {
				break
			}
			sleep := time.Duration(math.Min(8000, float64(500*(1<<attempt)))) * time.Millisecond
			time.Sleep(sleep)
		}
		if err != nil {
			return nil, err
		}
		results = append(results, batchResult...)
	}
	return results, nil
}

func (m *MemorySearchManager) lookupEmbeddingCache(ctx context.Context, hash string) ([]float64, bool) {
	var raw string
	row := m.db.QueryRow(ctx,
		`SELECT embedding FROM ai_memory_embedding_cache
         WHERE bridge_id=$1 AND provider=$2 AND model=$3 AND provider_key=$4 AND hash=$5`,
		m.bridgeID, m.status.Provider, m.status.Model, m.providerKey, hash,
	)
	if err := row.Scan(&raw); err != nil {
		return nil, false
	}
	embedding := parseEmbedding(raw)
	if len(embedding) == 0 {
		return nil, false
	}
	return embedding, true
}

func (m *MemorySearchManager) storeEmbeddingCache(ctx context.Context, hash string, embedding []float64) error {
	if len(embedding) == 0 {
		return nil
	}
	raw, _ := json.Marshal(embedding)
	dims := len(embedding)
	_, err := m.db.Exec(ctx,
		`INSERT INTO ai_memory_embedding_cache
         (bridge_id, provider, model, provider_key, hash, embedding, dims, updated_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
         ON CONFLICT (bridge_id, provider, model, provider_key, hash)
         DO UPDATE SET embedding=excluded.embedding, dims=excluded.dims, updated_at=excluded.updated_at`,
		m.bridgeID, m.status.Provider, m.status.Model, m.providerKey, hash, string(raw), dims, time.Now().UnixMilli(),
	)
	if err != nil {
		return err
	}
	maybePruneEmbeddingCache(ctx, m)
	return nil
}

func maybePruneEmbeddingCache(ctx context.Context, m *MemorySearchManager) {
	if m.cfg.Cache.MaxEntries <= 0 {
		return
	}
	var count int
	row := m.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM ai_memory_embedding_cache
         WHERE bridge_id=$1 AND provider=$2 AND model=$3 AND provider_key=$4`,
		m.bridgeID, m.status.Provider, m.status.Model, m.providerKey,
	)
	if err := row.Scan(&count); err != nil {
		return
	}
	if count <= m.cfg.Cache.MaxEntries {
		return
	}
	toRemove := count - m.cfg.Cache.MaxEntries
	_, _ = m.db.Exec(ctx,
		`DELETE FROM ai_memory_embedding_cache
         WHERE rowid IN (
           SELECT rowid FROM ai_memory_embedding_cache
           WHERE bridge_id=$1 AND provider=$2 AND model=$3 AND provider_key=$4
           ORDER BY updated_at ASC
           LIMIT $5
         )`,
		m.bridgeID, m.status.Provider, m.status.Model, m.providerKey, toRemove,
	)
}

func (m *MemorySearchManager) searchVector(ctx context.Context, queryVec []float64, limit int) ([]memory.HybridVectorResult, error) {
	if len(queryVec) == 0 || limit <= 0 {
		return nil, nil
	}
	baseArgs := []any{m.bridgeID, m.loginID, m.agentID, m.status.Model}
	filterSQL, filterArgs := sourceFilterSQL(5, m.cfg.Sources)
	args := append(baseArgs, filterArgs...)
	rows, err := m.db.Query(ctx,
		`SELECT id, path, start_line, end_line, text, embedding, source
         FROM ai_memory_chunks
         WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3 AND model=$4`+filterSQL,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		result memory.HybridVectorResult
		score  float64
	}
	var scoredResults []scored
	for rows.Next() {
		var id, path, text, embeddingRaw, source string
		var startLine, endLine int
		if err := rows.Scan(&id, &path, &startLine, &endLine, &text, &embeddingRaw, &source); err != nil {
			return nil, err
		}
		embedding := parseEmbedding(embeddingRaw)
		score := cosineSimilarity(queryVec, embedding)
		scoredResults = append(scoredResults, scored{
			result: memory.HybridVectorResult{
				ID:          id,
				Path:        path,
				StartLine:   startLine,
				EndLine:     endLine,
				Source:      source,
				Snippet:     truncateSnippet(text),
				VectorScore: score,
			},
			score: score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(scoredResults, func(i, j int) bool {
		return scoredResults[i].score > scoredResults[j].score
	})
	if len(scoredResults) > limit {
		scoredResults = scoredResults[:limit]
	}
	results := make([]memory.HybridVectorResult, 0, len(scoredResults))
	for _, entry := range scoredResults {
		results = append(results, entry.result)
	}
	return results, nil
}

func (m *MemorySearchManager) searchKeyword(ctx context.Context, query string, limit int) ([]memory.HybridKeywordResult, error) {
	if !m.ftsAvailable || limit <= 0 {
		return nil, nil
	}
	ftsQuery := memory.BuildFtsQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}
	baseArgs := []any{ftsQuery, m.status.Model, m.bridgeID, m.loginID, m.agentID}
	filterSQL, filterArgs := sourceFilterSQL(6, m.cfg.Sources)
	args := append(baseArgs, filterArgs...)
	rows, err := m.db.Query(ctx,
		`SELECT id, path, source, start_line, end_line, text,
           bm25(ai_memory_chunks_fts) AS rank
         FROM ai_memory_chunks_fts
         WHERE ai_memory_chunks_fts MATCH $1 AND model=$2 AND bridge_id=$3 AND login_id=$4 AND agent_id=$5`+filterSQL+`
         ORDER BY rank ASC
         LIMIT $`+fmt.Sprintf("%d", len(args)+1),
		append(args, limit)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []memory.HybridKeywordResult
	for rows.Next() {
		var id, path, source, text string
		var startLine, endLine int
		var rank float64
		if err := rows.Scan(&id, &path, &source, &startLine, &endLine, &text, &rank); err != nil {
			return nil, err
		}
		score := memory.BM25RankToScore(rank)
		results = append(results, memory.HybridKeywordResult{
			ID:        id,
			Path:      path,
			StartLine: startLine,
			EndLine:   endLine,
			Source:    source,
			Snippet:   truncateSnippet(text),
			TextScore: score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func sourceFilterSQL(startIndex int, sources []string) (string, []any) {
	if len(sources) == 0 {
		return "", nil
	}
	placeholders := make([]string, 0, len(sources))
	args := make([]any, 0, len(sources))
	for i, source := range sources {
		placeholders = append(placeholders, fmt.Sprintf("$%d", startIndex+i))
		args = append(args, source)
	}
	return " AND source IN (" + strings.Join(placeholders, ",") + ")", args
}

func parseEmbedding(raw string) []float64 {
	var values []float64
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	length := len(a)
	if len(b) < length {
		length = len(b)
	}
	var dot, normA, normB float64
	for i := 0; i < length; i++ {
		av := a[i]
		bv := b[i]
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func hasSource(sources []string, target string) bool {
	for _, source := range sources {
		if source == target {
			return true
		}
	}
	return false
}

func isExtraPath(path string, extra []string) bool {
	for _, extraPath := range extra {
		if strings.HasSuffix(strings.ToLower(extraPath), ".md") {
			if strings.EqualFold(path, extraPath) {
				return true
			}
			continue
		}
		if path == extraPath || strings.HasPrefix(path, extraPath+"/") {
			return true
		}
	}
	return false
}
