package error_recovery

import (
	_ "embed"
	"regexp"
	"strings"
	"sync"
)

//go:embed embed/SKILL.md
var rawSkillMD string

// parsedSkill 缓存 SKILL.md 解析结果
type parsedSkill struct {
	name        string
	description string
	body        string
	raw         string
}

var (
	parseOnce sync.Once
	parsed    parsedSkill
)

// SkillMD 返回 embed 的 SKILL.md 原文,供 loadSkill 内置短路直接返回
func SkillMD() string {
	ensureParsed()
	return parsed.raw
}

// SkillName 返回 frontmatter 中的 name
func SkillName() string {
	ensureParsed()
	return parsed.name
}

// SkillDescription 返回 frontmatter 中的 description(供 buildSkillsSystemPrompt 拼接)
func SkillDescription() string {
	ensureParsed()
	return parsed.description
}

func ensureParsed() {
	parseOnce.Do(func() {
		parsed = parseSkillMD(rawSkillMD)
	})
}

// parseSkillMD 解析 YAML frontmatter 中的 name / description,兼容多行块标量
// 参考 pkg/skillmd 解析器,但本包内联实现避免循环依赖
var (
	nameRegexp = regexp.MustCompile(`(?m)^(?:#{1,2}[ \t]+)?name:[ \t]*(.+)$`)
	descRegexp = regexp.MustCompile(`(?m)^(?:#{1,2}[ \t]+)?description:[ \t]*(.*)$`)
)

func parseSkillMD(raw string) parsedSkill {
	res := parsedSkill{raw: raw}

	fmStart := strings.Index(raw, "---")
	if fmStart < 0 {
		res.body = raw
		return res
	}
	fmEndRel := strings.Index(raw[fmStart+3:], "---")
	if fmEndRel < 0 {
		res.body = raw
		return res
	}
	fm := raw[fmStart+3 : fmStart+3+fmEndRel]
	res.body = strings.TrimLeft(raw[fmStart+3+fmEndRel+3:], "\r\n")

	if m := nameRegexp.FindStringSubmatch(fm); len(m) == 2 {
		res.name = strings.TrimSpace(m[1])
	}
	res.description = extractDescription(fm)
	return res
}

// extractDescription 提取 description 字段值,支持 `>`, `>-`, `|`, `|-` 块标量
func extractDescription(fm string) string {
	lines := strings.Split(fm, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, "# \t")
		if !strings.HasPrefix(trimmed, "description:") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
		// 单行值
		if rest != "" && !isBlockScalarMarker(rest) {
			return rest
		}
		// 块标量:收集后续缩进行
		var buf strings.Builder
		for j := i + 1; j < len(lines); j++ {
			l := lines[j]
			if len(l) == 0 {
				buf.WriteString(" ")
				continue
			}
			// 缩进行(2 空格及以上)属于块标量内容
			if l[0] != ' ' && l[0] != '\t' {
				break
			}
			buf.WriteString(strings.TrimSpace(l))
			buf.WriteString(" ")
		}
		return strings.TrimSpace(buf.String())
	}
	return ""
}

func isBlockScalarMarker(s string) bool {
	s = strings.TrimSpace(s)
	return s == ">" || s == ">-" || s == "|" || s == "|-"
}
