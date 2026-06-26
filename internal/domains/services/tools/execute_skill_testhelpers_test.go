package tools

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/agents"
)

// fileEntry 测试辅助：用 (path, key) 简化 SkillFile 构造
type fileEntry struct {
	path string
	key  string
}

func skillRefForTest(skillID, version string) agents.SkillRef {
	return agents.SkillRef{
		SkillID:   skillID,
		Version:   version,
		SkillName: "test-skill",
	}
}

func skillVersionDOForTest(files []fileEntry) models.SkillVersionDO {
	sf := make([]models.SkillFile, 0, len(files))
	for _, f := range files {
		sf = append(sf, models.SkillFile{
			Path:    f.path,
			FileKey: f.key,
		})
	}
	return models.SkillVersionDO{
		SkillFiles: sf,
	}
}
