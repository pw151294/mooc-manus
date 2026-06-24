package repositories

import (
	"errors"

	domainModels "mooc-manus/internal/domains/models"
	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"gorm.io/gorm"
)

type SkillVersionRepository interface {
	Create(po models.SkillVersionPO) error
	Update(po models.SkillVersionPO) error
	DeleteById(id string) error
	DeleteBySkillID(skillId string) error
	GetById(id string) (models.SkillVersionPO, error)
	GetBySkillIDAndVersion(skillId, version string) (models.SkillVersionPO, bool, error)
	GetDraftBySkillID(skillId string) (models.SkillVersionPO, bool, error)
	ListBySkillID(skillId string) ([]models.SkillVersionPO, error)
	ListReleasedBySkillID(skillId string) ([]models.SkillVersionPO, error)
	ListReleasedVersionStrings(skillId string) ([]string, error)
	BatchLatestBySkillIDs(skillIds []string) (map[string]models.SkillVersionPO, error)
}

type SkillVersionRepositoryImpl struct {
	client *gorm.DB
}

func NewSkillVersionRepository() SkillVersionRepository {
	return &SkillVersionRepositoryImpl{client: storage.GetPostgresClient()}
}

func (r *SkillVersionRepositoryImpl) Create(po models.SkillVersionPO) error {
	return r.client.Create(&po).Error
}

func (r *SkillVersionRepositoryImpl) Update(po models.SkillVersionPO) error {
	return r.client.Model(&po).Where("skill_version_id = ?", po.ID).Updates(po).Error
}

func (r *SkillVersionRepositoryImpl) DeleteById(id string) error {
	return r.client.Where("skill_version_id = ?", id).Delete(&models.SkillVersionPO{}).Error
}

func (r *SkillVersionRepositoryImpl) DeleteBySkillID(skillId string) error {
	return r.client.Where("skill_id = ?", skillId).Delete(&models.SkillVersionPO{}).Error
}

func (r *SkillVersionRepositoryImpl) GetById(id string) (models.SkillVersionPO, error) {
	var po models.SkillVersionPO
	err := r.client.Where("skill_version_id = ?", id).First(&po).Error
	return po, err
}

// GetBySkillIDAndVersion 按 (skill_id, version) 联合唯一查询；不存在返回 (zero, false, nil)
func (r *SkillVersionRepositoryImpl) GetBySkillIDAndVersion(skillId, version string) (models.SkillVersionPO, bool, error) {
	var po models.SkillVersionPO
	err := r.client.Where("skill_id = ? AND version = ?", skillId, version).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return po, false, nil
		}
		return po, false, err
	}
	return po, true, nil
}

// GetDraftBySkillID 同一 Skill 的 draft 版本最多 1 份
func (r *SkillVersionRepositoryImpl) GetDraftBySkillID(skillId string) (models.SkillVersionPO, bool, error) {
	return r.GetBySkillIDAndVersion(skillId, domainModels.SkillDraftVersionString)
}

// ListBySkillID 列出 skill 全部版本（含 draft），按 created_at DESC 排序
func (r *SkillVersionRepositoryImpl) ListBySkillID(skillId string) ([]models.SkillVersionPO, error) {
	var pos []models.SkillVersionPO
	err := r.client.Where("skill_id = ?", skillId).
		Order("created_at DESC").
		Find(&pos).Error
	return pos, err
}

// ListReleasedBySkillID 列出 skill 全部正式版本（排除 draft），按 created_at DESC 排序
// 业务规则 §5.13.4
func (r *SkillVersionRepositoryImpl) ListReleasedBySkillID(skillId string) ([]models.SkillVersionPO, error) {
	var pos []models.SkillVersionPO
	err := r.client.Where("skill_id = ? AND version <> ?", skillId, domainModels.SkillDraftVersionString).
		Order("created_at DESC").
		Find(&pos).Error
	return pos, err
}

// ListReleasedVersionStrings 仅返回 skill 已发布版本的版本号字符串数组
// 业务规则 §5.4：发布时取该 Skill 所有 v 开头版本号求最大
func (r *SkillVersionRepositoryImpl) ListReleasedVersionStrings(skillId string) ([]string, error) {
	var versions []string
	err := r.client.Model(&models.SkillVersionPO{}).
		Where("skill_id = ? AND version <> ?", skillId, domainModels.SkillDraftVersionString).
		Pluck("version", &versions).Error
	return versions, err
}

// BatchLatestBySkillIDs 按 skill_id 列表批量查询最新已发布版本（每个 skill 取 created_at DESC 第一条）
// 业务规则 §5.8：列表组装 latestVersion 字段时使用
func (r *SkillVersionRepositoryImpl) BatchLatestBySkillIDs(skillIds []string) (map[string]models.SkillVersionPO, error) {
	result := make(map[string]models.SkillVersionPO)
	if len(skillIds) == 0 {
		return result, nil
	}
	// 简单实现：逐个查询。skill 数量通常 ≤ pageSize（默认 10），可接受
	for _, sid := range skillIds {
		var po models.SkillVersionPO
		err := r.client.
			Where("skill_id = ? AND version <> ?", sid, domainModels.SkillDraftVersionString).
			Order("created_at DESC").
			First(&po).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, err
		}
		result[sid] = po
	}
	return result, nil
}
