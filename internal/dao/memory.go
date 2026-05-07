package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

type MemoryDAO struct {
	db *gorm.DB
}

func NewMemoryDAO(db *gorm.DB) *MemoryDAO {
	return &MemoryDAO{db: db}
}

func (d *MemoryDAO) activeScope() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_deleted = ?", false)
	}
}

func (d *MemoryDAO) Create(ctx context.Context, memory *model.Memory) error {
	return d.db.WithContext(ctx).Create(memory).Error
}

func (d *MemoryDAO) Update(ctx context.Context, memory *model.Memory) error {
	return d.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", memory.ID, memory.UserID).
		Updates(memory).Error
}

func (d *MemoryDAO) GetByID(ctx context.Context, userID, memoryID uint64) (*model.Memory, error) {
	var memory model.Memory
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("id = ? AND user_id = ?", memoryID, userID).
		First(&memory).Error
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

func (d *MemoryDAO) ListByLayer(ctx context.Context, userID uint64, layer model.MemoryLayer) ([]model.Memory, error) {
	var memories []model.Memory
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("user_id = ? AND layer = ?", userID, layer).
		Order("updated_at DESC").
		Find(&memories).Error
	return memories, err
}

func (d *MemoryDAO) SearchByFactType(ctx context.Context, userID uint64, factType string) ([]model.Memory, error) {
	var memories []model.Memory
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("user_id = ? AND layer = ? AND fact_type = ?", userID, model.MemoryLayerL2, factType).
		Find(&memories).Error
	return memories, err
}

func (d *MemoryDAO) SearchByContent(ctx context.Context, userID uint64, layer model.MemoryLayer, keyword string) ([]model.Memory, error) {
	var memories []model.Memory
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("user_id = ? AND layer = ? AND content LIKE ?", userID, layer, "%"+keyword+"%").
		Order("updated_at DESC").
		Limit(20).
		Find(&memories).Error
	return memories, err
}

func (d *MemoryDAO) Delete(ctx context.Context, userID, memoryID uint64) error {
	return d.db.WithContext(ctx).
		Model(&model.Memory{}).
		Where("id = ? AND user_id = ?", memoryID, userID).
		Update("is_deleted", true).Error
}

func (d *MemoryDAO) GetByIDs(ctx context.Context, userID uint64, ids []uint64) ([]model.Memory, error) {
	var memories []model.Memory
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("user_id = ? AND id IN ?", userID, ids).
		Find(&memories).Error
	return memories, err
}

func (d *MemoryDAO) ListWithoutEmbedding(ctx context.Context) ([]model.Memory, error) {
	var memories []model.Memory
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("layer IN ? AND (embedding_id = '' OR embedding_id IS NULL)", []string{"L2", "L3"}).
		Limit(100).
		Find(&memories).Error
	return memories, err
}
