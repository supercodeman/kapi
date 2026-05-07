package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

type SkillDAO struct {
	db *gorm.DB
}

func NewSkillDAO(db *gorm.DB) *SkillDAO {
	return &SkillDAO{db: db}
}

func (d *SkillDAO) ListEnabled(ctx context.Context) ([]model.Skill, error) {
	var skills []model.Skill
	err := d.db.WithContext(ctx).
		Where("status = ?", "enabled").
		Find(&skills).Error
	return skills, err
}

func (d *SkillDAO) GetByName(ctx context.Context, name string) (*model.Skill, error) {
	var s model.Skill
	err := d.db.WithContext(ctx).Where("name = ?", name).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *SkillDAO) List(ctx context.Context) ([]model.Skill, error) {
	var skills []model.Skill
	err := d.db.WithContext(ctx).Order("name").Find(&skills).Error
	return skills, err
}

func (d *SkillDAO) Create(ctx context.Context, s *model.Skill) error {
	return d.db.WithContext(ctx).Create(s).Error
}

func (d *SkillDAO) Update(ctx context.Context, s *model.Skill) error {
	return d.db.WithContext(ctx).Save(s).Error
}

func (d *SkillDAO) Delete(ctx context.Context, name string) error {
	return d.db.WithContext(ctx).Where("name = ?", name).Delete(&model.Skill{}).Error
}
