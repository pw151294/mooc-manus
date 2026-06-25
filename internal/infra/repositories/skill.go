package repositories

import (
	"errors"
	"strings"

	"mooc-manus/internal/infra/models"
	"mooc-manus/internal/infra/storage"

	"gorm.io/gorm"
)

// SkillListFilter Repository 层用于分页/全量查询的过滤条件（与 DTO 解耦）
type SkillListFilter struct {
	ProviderID string
	NameLike   string // 同时匹配 skillName 与 keyword
	Status     string
}

type SkillRepository interface {
	Create(po models.SkillPO) error
	Update(po models.SkillPO) error
	DeleteById(id string) error
	GetById(id string) (models.SkillPO, error)
	GetByName(name string) (models.SkillPO, bool, error)
	GetByIds(ids []string) ([]models.SkillPO, error)
	GetByNames(names []string) ([]models.SkillPO, error)
	Page(filter SkillListFilter, pageNum, pageSize int) ([]models.SkillPO, int64, error)
	List(filter SkillListFilter) ([]models.SkillPO, error)
	CountByProviderId(providerId string) (int64, error)
	UpdateLatestVersionId(skillId, versionId string) error
	ClearLatestVersionId(skillId string) error
	Transaction(fn func(txSkillRepo SkillRepository, txSkillVersionRepo SkillVersionRepository, txSkillProviderRepo SkillProviderRepository) error) error
}

type SkillRepositoryImpl struct {
	client *gorm.DB
}

func NewSkillRepository() SkillRepository {
	return &SkillRepositoryImpl{client: storage.GetPostgresClient()}
}

func (r *SkillRepositoryImpl) Create(po models.SkillPO) error {
	return r.client.Create(&po).Error
}

func (r *SkillRepositoryImpl) Update(po models.SkillPO) error {
	return r.client.Model(&po).Where("skill_id = ?", po.ID).Updates(po).Error
}

func (r *SkillRepositoryImpl) DeleteById(id string) error {
	return r.client.Where("skill_id = ?", id).Delete(&models.SkillPO{}).Error
}

func (r *SkillRepositoryImpl) GetById(id string) (models.SkillPO, error) {
	var po models.SkillPO
	err := r.client.Where("skill_id = ?", id).First(&po).Error
	return po, err
}

// GetByName 按 skill_name 全局唯一查询；不存在返回 (zero, false, nil)
func (r *SkillRepositoryImpl) GetByName(name string) (models.SkillPO, bool, error) {
	var po models.SkillPO
	err := r.client.Where("skill_name = ?", name).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return po, false, nil
		}
		return po, false, err
	}
	return po, true, nil
}

func (r *SkillRepositoryImpl) GetByIds(ids []string) ([]models.SkillPO, error) {
	var pos []models.SkillPO
	if len(ids) == 0 {
		return pos, nil
	}
	err := r.client.Where("skill_id IN ?", ids).Find(&pos).Error
	return pos, err
}

// GetByNames 批量按 skill_name 查询（用于 Skill 系统提示词拼接）
// 不存在的 name 会被跳过，返回查询到的部分结果
func (r *SkillRepositoryImpl) GetByNames(names []string) ([]models.SkillPO, error) {
	var pos []models.SkillPO
	if len(names) == 0 {
		return pos, nil
	}
	err := r.client.Where("skill_name IN ?", names).Find(&pos).Error
	return pos, err
}

// applyFilter 将过滤条件叠加到查询链，并返回叠加后的 *gorm.DB
func (r *SkillRepositoryImpl) applyFilter(tx *gorm.DB, filter SkillListFilter) *gorm.DB {
	if filter.ProviderID != "" {
		tx = tx.Where("skill_provider_id = ?", filter.ProviderID)
	}
	if filter.Status != "" {
		tx = tx.Where("status = ?", filter.Status)
	}
	if filter.NameLike != "" {
		// LIKE %name%，业务规则要求截断到 64 字符（由 Service 层完成）
		like := "%" + strings.ReplaceAll(filter.NameLike, "%", `\%`) + "%"
		tx = tx.Where("skill_name LIKE ?", like)
	}
	return tx
}

// Page 业务规则 §5.8：排序固定 updated_at DESC, skill_id DESC
func (r *SkillRepositoryImpl) Page(filter SkillListFilter, pageNum, pageSize int) ([]models.SkillPO, int64, error) {
	var (
		pos   []models.SkillPO
		total int64
	)
	countTx := r.applyFilter(r.client.Model(&models.SkillPO{}), filter)
	if err := countTx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return pos, 0, nil
	}
	listTx := r.applyFilter(r.client.Model(&models.SkillPO{}), filter)
	offset := (pageNum - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	err := listTx.Order("updated_at DESC").Order("skill_id DESC").
		Offset(offset).Limit(pageSize).Find(&pos).Error
	return pos, total, err
}

// List 全量返回（不分页），排序与 Page 一致
func (r *SkillRepositoryImpl) List(filter SkillListFilter) ([]models.SkillPO, error) {
	var pos []models.SkillPO
	listTx := r.applyFilter(r.client.Model(&models.SkillPO{}), filter)
	err := listTx.Order("updated_at DESC").Order("skill_id DESC").Find(&pos).Error
	return pos, err
}

func (r *SkillRepositoryImpl) CountByProviderId(providerId string) (int64, error) {
	var count int64
	err := r.client.Model(&models.SkillPO{}).Where("skill_provider_id = ?", providerId).Count(&count).Error
	return count, err
}

func (r *SkillRepositoryImpl) UpdateLatestVersionId(skillId, versionId string) error {
	return r.client.Model(&models.SkillPO{}).
		Where("skill_id = ?", skillId).
		Update("latest_version_id", versionId).Error
}

func (r *SkillRepositoryImpl) ClearLatestVersionId(skillId string) error {
	return r.client.Model(&models.SkillPO{}).
		Where("skill_id = ?", skillId).
		Update("latest_version_id", "").Error
}

// Transaction 在同一个事务内提供 Skill / SkillVersion / SkillProvider 三个 Repository
// 用于发布、回滚、删除 Skill 等跨表原子操作
func (r *SkillRepositoryImpl) Transaction(fn func(
	txSkillRepo SkillRepository,
	txSkillVersionRepo SkillVersionRepository,
	txSkillProviderRepo SkillProviderRepository,
) error) error {
	return r.client.Transaction(func(tx *gorm.DB) error {
		txSkillRepo := &SkillRepositoryImpl{client: tx}
		txSkillVersionRepo := &SkillVersionRepositoryImpl{client: tx}
		txSkillProviderRepo := &SkillProviderRepositoryImpl{client: tx}
		return fn(txSkillRepo, txSkillVersionRepo, txSkillProviderRepo)
	})
}
