package memory

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

type MemoryManager struct {
	memoryDAO *dao.MemoryDAO
	milvus    *MilvusClient
	embedding EmbeddingProvider
	convStore *ConversationStore
}

func NewMemoryManager(memoryDAO *dao.MemoryDAO, milvus *MilvusClient, embedding EmbeddingProvider, convStore *ConversationStore) *MemoryManager {
	return &MemoryManager{
		memoryDAO: memoryDAO,
		milvus:    milvus,
		embedding: embedding,
		convStore: convStore,
	}
}

func (m *MemoryManager) SaveMemory(ctx context.Context, memory *model.Memory) error {
	if err := m.memoryDAO.Create(ctx, memory); err != nil {
		return err
	}

	if memory.Layer == model.MemoryLayerL2 {
		m.embedSync(ctx, memory)
	} else if memory.Layer == model.MemoryLayerL3 {
		go m.embedAsync(memory)
	}
	return nil
}

func (m *MemoryManager) UpdateMemory(ctx context.Context, memory *model.Memory) error {
	if err := m.memoryDAO.Update(ctx, memory); err != nil {
		return err
	}

	if memory.Layer == model.MemoryLayerL2 || memory.Layer == model.MemoryLayerL3 {
		if memory.EmbeddingID != "" && m.milvus != nil {
			m.milvus.DeleteByMemoryID(ctx, int64(memory.ID))
		}
		if memory.Layer == model.MemoryLayerL2 {
			m.embedSync(ctx, memory)
		} else {
			go m.embedAsync(memory)
		}
	}
	return nil
}

func (m *MemoryManager) DeleteMemory(ctx context.Context, userID, memoryID uint64) error {
	mem, err := m.memoryDAO.GetByID(ctx, userID, memoryID)
	if err != nil {
		return err
	}
	if mem.EmbeddingID != "" && m.milvus != nil {
		m.milvus.DeleteByMemoryID(ctx, int64(memoryID))
	}
	return m.memoryDAO.Delete(ctx, userID, memoryID)
}

func (m *MemoryManager) GetUserProfile(ctx context.Context, userID uint64) ([]model.Memory, error) {
	return m.memoryDAO.ListByLayer(ctx, userID, model.MemoryLayerL1)
}

func (m *MemoryManager) SearchFacts(ctx context.Context, userID uint64, query string, topK int) ([]model.Memory, error) {
	if m.embedding == nil || m.milvus == nil {
		return m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL2, query)
	}

	vectors, err := m.embedding.Embed(ctx, []string{query})
	if err != nil {
		log.Printf("embedding failed, falling back to keyword search: %v", err)
		return m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL2, query)
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL2, query)
	}

	memoryIDs, _, err := m.milvus.Search(ctx, int64(userID), vectors[0], topK)
	if err != nil {
		log.Printf("milvus search failed, falling back to keyword search: %v", err)
		return m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL2, query)
	}

	if len(memoryIDs) == 0 {
		return nil, nil
	}

	uintIDs := make([]uint64, len(memoryIDs))
	for i, id := range memoryIDs {
		uintIDs[i] = uint64(id)
	}
	return m.memoryDAO.GetByIDs(ctx, userID, uintIDs)
}

func (m *MemoryManager) SearchEpisodes(ctx context.Context, userID uint64, query string, topK int) ([]model.Memory, error) {
	if m.embedding == nil || m.milvus == nil {
		return m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL3, query)
	}

	vectors, err := m.embedding.Embed(ctx, []string{query})
	if err != nil {
		return m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL3, query)
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL3, query)
	}

	memoryIDs, _, err := m.milvus.Search(ctx, int64(userID), vectors[0], topK)
	if err != nil {
		return m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL3, query)
	}

	if len(memoryIDs) == 0 {
		return nil, nil
	}

	uintIDs := make([]uint64, len(memoryIDs))
	for i, id := range memoryIDs {
		uintIDs[i] = uint64(id)
	}
	return m.memoryDAO.GetByIDs(ctx, userID, uintIDs)
}

func (m *MemoryManager) GetRecentConversation(ctx context.Context, userID, sessionID uint64, limit int) ([]ConversationMessage, error) {
	return m.convStore.GetRecent(ctx, userID, sessionID, limit)
}

// SearchFactsAndEpisodes 单次 Embedding + 并行 Milvus 搜索 L2/L3
func (m *MemoryManager) SearchFactsAndEpisodes(ctx context.Context, userID uint64, query string, factsTopK, episodesTopK int) ([]model.Memory, []model.Memory, error) {
	if m.embedding == nil || m.milvus == nil {
		var facts, episodes []model.Memory
		var fErr, eErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			facts, fErr = m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL2, query)
		}()
		go func() {
			defer wg.Done()
			episodes, eErr = m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL3, query)
		}()
		wg.Wait()
		if fErr != nil {
			return nil, episodes, fErr
		}
		return facts, episodes, eErr
	}

	vectors, err := m.embedding.Embed(ctx, []string{query})
	if err != nil || len(vectors) == 0 || len(vectors[0]) == 0 {
		log.Printf("shared embedding failed, falling back to keyword search: %v", err)
		var facts, episodes []model.Memory
		var fErr, eErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			facts, fErr = m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL2, query)
		}()
		go func() {
			defer wg.Done()
			episodes, eErr = m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL3, query)
		}()
		wg.Wait()
		if fErr != nil {
			return nil, episodes, fErr
		}
		return facts, episodes, eErr
	}

	vec := vectors[0]
	var facts, episodes []model.Memory
	var fErr, eErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		memoryIDs, _, searchErr := m.milvus.Search(ctx, int64(userID), vec, factsTopK)
		if searchErr != nil {
			log.Printf("milvus search L2 failed, falling back: %v", searchErr)
			facts, fErr = m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL2, query)
			return
		}
		if len(memoryIDs) == 0 {
			return
		}
		uintIDs := make([]uint64, len(memoryIDs))
		for i, id := range memoryIDs {
			uintIDs[i] = uint64(id)
		}
		facts, fErr = m.memoryDAO.GetByIDs(ctx, userID, uintIDs)
	}()

	go func() {
		defer wg.Done()
		memoryIDs, _, searchErr := m.milvus.Search(ctx, int64(userID), vec, episodesTopK)
		if searchErr != nil {
			log.Printf("milvus search L3 failed, falling back: %v", searchErr)
			episodes, eErr = m.memoryDAO.SearchByContent(ctx, userID, model.MemoryLayerL3, query)
			return
		}
		if len(memoryIDs) == 0 {
			return
		}
		uintIDs := make([]uint64, len(memoryIDs))
		for i, id := range memoryIDs {
			uintIDs[i] = uint64(id)
		}
		episodes, eErr = m.memoryDAO.GetByIDs(ctx, userID, uintIDs)
	}()

	wg.Wait()
	if fErr != nil {
		return nil, episodes, fErr
	}
	return facts, episodes, eErr
}

func (m *MemoryManager) AddConversationMessage(ctx context.Context, userID, sessionID uint64, msg *ConversationMessage) error {
	return m.convStore.AddMessage(ctx, userID, sessionID, msg)
}

// BackfillEmbeddings 补偿历史记忆的 Embedding（embedding_id 为空的 L2/L3 记忆）
func (m *MemoryManager) BackfillEmbeddings(ctx context.Context) {
	if m.embedding == nil || m.milvus == nil {
		return
	}
	memories, err := m.memoryDAO.ListWithoutEmbedding(ctx)
	if err != nil {
		log.Printf("backfill: failed to list memories without embedding: %v", err)
		return
	}
	if len(memories) == 0 {
		return
	}
	log.Printf("backfill: found %d memories without embedding, processing...", len(memories))
	for _, mem := range memories {
		m.embedSync(ctx, &mem)
	}
	log.Printf("backfill: completed")
}

func (m *MemoryManager) embedSync(ctx context.Context, memory *model.Memory) {
	if m.embedding == nil || m.milvus == nil {
		return
	}
	vectors, err := m.embedding.Embed(ctx, []string{memory.Content})
	if err != nil {
		log.Printf("sync embedding failed for memory %d: %v", memory.ID, err)
		return
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		log.Printf("sync embedding returned empty for memory %d", memory.ID)
		return
	}
	if err := m.milvus.Insert(ctx, int64(memory.ID), int64(memory.UserID), vectors[0]); err != nil {
		log.Printf("milvus insert failed for memory %d: %v", memory.ID, err)
		return
	}
	memory.EmbeddingID = "milvus"
	if err := m.memoryDAO.Update(ctx, memory); err != nil {
		log.Printf("failed to update embedding_id for memory %d: %v", memory.ID, err)
	}
}

func (m *MemoryManager) embedAsync(memory *model.Memory) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("embedAsync panic for memory %d: %v", memory.ID, r)
		}
	}()
	m.embedSync(ctx, memory)
}
