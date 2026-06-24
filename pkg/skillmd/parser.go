// Package skillmd 提供 SKILL.md frontmatter 元数据的解析能力。
//
// 行为严格对齐 beedance Java 端 SkillMetadataParser.parse 与 SkillService.parseSkillMdMetadata：
//   - YAML frontmatter 必须以 --- 起止
//   - name / description 用同一组多行正则提取，兼容前缀 # / ## 与 description 的块标量语法
//   - 任一字段解析为空均视为参数错误
//
// 所有对外错误均 wrap skillerr.ErrInvalidInput，Handler 层无需新增映射。
package skillmd

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"regexp"
	"strings"

	"mooc-manus/pkg/skillerr"
)

// SkillMdFilename Java 端 SkillConstants.SKILL_MD_FILENAME 对应值；严格大小写敏感。
const SkillMdFilename = "SKILL.md"

// Metadata 对齐 Java SkillMetadata：本期仅包含 name / description。
type Metadata struct {
	Name        string
	Description string
}

// 哨兵错误。Handler 层通过 errors.Is(err, skillerr.ErrInvalidInput) 统一映射 HTTP 400。
var (
	ErrSkillMdMissing     = errors.New("SKILL.md missing")
	ErrInvalidSkillMd     = errors.New("SKILL.md format invalid")
	ErrSkillMdNameMissing = errors.New("SKILL.md name field missing")
	ErrSkillMdDescMissing = errors.New("SKILL.md description field missing")
)

// 多行模式 + 兼容 "# " / "## " 标题前缀，与 Java NAME_PATTERN / DESCRIPTION_PATTERN 等价。
var (
	nameRegexp = regexp.MustCompile(`(?m)^(?:#{1,2}[ \t]+)?name:[ \t]*(.+)$`)
	descRegexp = regexp.MustCompile(`(?m)^(?:#{1,2}[ \t]+)?description:[ \t]*(.+)$`)
)

// blockScalarMarkers 与 Java extractDescription 中四个常量一致。
var blockScalarMarkers = map[string]struct{}{
	">-": {}, ">": {}, "|-": {}, "|": {},
}

// Parse 解析 SKILL.md 全文，返回 frontmatter 中的 name / description。
//
// 行为对齐 Java SkillMetadataParser.parse：
//   - frontmatter 缺失 / 未闭合 → 返回 ErrInvalidSkillMd（wrap skillerr.ErrInvalidInput）
//   - 字段未匹配 → 返回空串而非错误（blank 校验由 ExtractFromUploads 负责）
func Parse(content string) (Metadata, error) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return Metadata{}, wrapInvalid(ErrInvalidSkillMd, "missing YAML frontmatter")
	}
	endIndex := strings.Index(trimmed[3:], "---")
	if endIndex == -1 {
		return Metadata{}, wrapInvalid(ErrInvalidSkillMd, "YAML frontmatter not closed")
	}
	yaml := strings.TrimSpace(trimmed[3 : 3+endIndex])

	return Metadata{
		Name:        extractField(yaml, nameRegexp),
		Description: extractDescription(yaml),
	}, nil
}

// ExtractFromUploads 对齐 Java SkillService.parseSkillMdMetadata。
//
//   - files 中按严格大小写匹配 Filename == "SKILL.md"，取第一个；不递归子路径。
//   - required=true：未上传任何文件 / 未找到 SKILL.md → ErrSkillMdMissing
//   - required=false：未找到时返回 (Metadata{}, false, nil) 让上层沿用旧值
//   - 解析成功后若 name / description 任一为空，分别返回对应哨兵错误
//
// 返回值 found 表示 SKILL.md 文件本身是否存在并被解析（与是否覆盖业务字段对应）。
func ExtractFromUploads(files []*multipart.FileHeader, required bool) (Metadata, bool, error) {
	if len(files) == 0 {
		if required {
			return Metadata{}, false, wrapInvalid(ErrSkillMdMissing, "no file uploaded")
		}
		return Metadata{}, false, nil
	}

	var target *multipart.FileHeader
	for _, fh := range files {
		if fh != nil && fh.Filename == SkillMdFilename {
			target = fh
			break
		}
	}
	if target == nil {
		if required {
			return Metadata{}, false, wrapInvalid(ErrSkillMdMissing, "SKILL.md not found in uploaded files")
		}
		return Metadata{}, false, nil
	}

	f, err := target.Open()
	if err != nil {
		return Metadata{}, false, wrapInvalid(ErrInvalidSkillMd, "open SKILL.md failed: "+err.Error())
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return Metadata{}, false, wrapInvalid(ErrInvalidSkillMd, "read SKILL.md failed: "+err.Error())
	}

	md, err := Parse(string(raw))
	if err != nil {
		return Metadata{}, false, err
	}
	if strings.TrimSpace(md.Name) == "" {
		return Metadata{}, true, wrapInvalid(ErrSkillMdNameMissing, "name field is blank")
	}
	if strings.TrimSpace(md.Description) == "" {
		return Metadata{}, true, wrapInvalid(ErrSkillMdDescMissing, "description field is blank")
	}
	return md, true, nil
}

// extractField 与 Java extractField 一致：未匹配返回空串。
func extractField(yaml string, pattern *regexp.Regexp) string {
	m := pattern.FindStringSubmatch(yaml)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// extractDescription 复刻 Java extractDescription 的块标量分支。
func extractDescription(yaml string) string {
	loc := descRegexp.FindStringSubmatchIndex(yaml)
	if loc == nil {
		return ""
	}
	value := strings.TrimSpace(yaml[loc[2]:loc[3]])
	if _, isBlock := blockScalarMarkers[value]; !isBlock {
		return value
	}

	// 块标量：从 description 行末尾开始扫描后续行，收集以 "  " 或 "\t" 开头的行
	descEnd := loc[1]
	remaining := yaml[descEnd:]
	var sb strings.Builder
	for _, line := range strings.Split(remaining, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(strings.TrimSpace(line))
			continue
		}
		break
	}
	result := strings.TrimSpace(sb.String())
	if result == "" {
		// Java 端会 warn 后返回原始符号，此处保留同样语义
		return value
	}
	return result
}

func wrapInvalid(sentinel error, detail string) error {
	if detail == "" {
		return fmt.Errorf("%w: %w", sentinel, skillerr.ErrInvalidInput)
	}
	return fmt.Errorf("%w: %s: %w", sentinel, detail, skillerr.ErrInvalidInput)
}
