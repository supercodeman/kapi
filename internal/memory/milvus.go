package memory

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	collectionName = "memories_vectors"
	embeddingDim   = 1024
	topKDefault    = 10
)

type MilvusClient struct {
	client client.Client
}

func NewMilvusClient(addr string) (*MilvusClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := client.NewClient(ctx, client.Config{
		Address: addr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect milvus: %w", err)
	}
	return &MilvusClient{client: c}, nil
}

func (m *MilvusClient) EnsureCollection(ctx context.Context) error {
	has, err := m.client.HasCollection(ctx, collectionName)
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	schema := &entity.Schema{
		CollectionName: collectionName,
		Fields: []*entity.Field{
			{Name: "id", DataType: entity.FieldTypeInt64, PrimaryKey: true, AutoID: true},
			{Name: "memory_id", DataType: entity.FieldTypeInt64},
			{Name: "user_id", DataType: entity.FieldTypeInt64},
			{Name: "embedding", DataType: entity.FieldTypeFloatVector, TypeParams: map[string]string{"dim": fmt.Sprintf("%d", embeddingDim)}},
		},
	}

	if err := m.client.CreateCollection(ctx, schema, entity.DefaultShardNumber); err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	idx, err := entity.NewIndexIvfFlat(entity.L2, 128)
	if err != nil {
		return err
	}
	if err := m.client.CreateIndex(ctx, collectionName, "embedding", idx, false); err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	if err := m.client.LoadCollection(ctx, collectionName, false); err != nil {
		return fmt.Errorf("failed to load collection: %w", err)
	}

	log.Printf("milvus collection %s created and loaded", collectionName)
	return nil
}

func (m *MilvusClient) Insert(ctx context.Context, memoryID, userID int64, embedding []float32) error {
	memoryIDCol := entity.NewColumnInt64("memory_id", []int64{memoryID})
	userIDCol := entity.NewColumnInt64("user_id", []int64{userID})
	embeddingCol := entity.NewColumnFloatVector("embedding", embeddingDim, [][]float32{embedding})

	_, err := m.client.Insert(ctx, collectionName, "", memoryIDCol, userIDCol, embeddingCol)
	return err
}

func (m *MilvusClient) Search(ctx context.Context, userID int64, queryVector []float32, topK int) ([]int64, []float32, error) {
	if topK <= 0 {
		topK = topKDefault
	}

	sp, err := entity.NewIndexIvfFlatSearchParam(16)
	if err != nil {
		return nil, nil, err
	}

	results, err := m.client.Search(
		ctx,
		collectionName,
		nil,
		fmt.Sprintf("user_id == %d", userID),
		[]string{"memory_id"},
		[]entity.Vector{entity.FloatVector(queryVector)},
		"embedding",
		entity.L2,
		topK,
		sp,
	)
	if err != nil {
		return nil, nil, err
	}

	if len(results) == 0 {
		return nil, nil, nil
	}

	var memoryIDs []int64
	var scores []float32
	for _, result := range results {
		if result.ResultCount == 0 {
			continue
		}
		col := result.Fields.GetColumn("memory_id")
		if col == nil {
			continue
		}
		memoryIDCol, ok := col.(*entity.ColumnInt64)
		if !ok {
			continue
		}
		for i := 0; i < result.ResultCount; i++ {
			val, err := memoryIDCol.ValueByIdx(i)
			if err != nil {
				continue
			}
			memoryIDs = append(memoryIDs, val)
			scores = append(scores, result.Scores[i])
		}
	}

	return memoryIDs, scores, nil
}

func (m *MilvusClient) DeleteByMemoryID(ctx context.Context, memoryID int64) error {
	return m.client.Delete(ctx, collectionName, "", fmt.Sprintf("memory_id == %d", memoryID))
}

func (m *MilvusClient) Close() error {
	return m.client.Close()
}
