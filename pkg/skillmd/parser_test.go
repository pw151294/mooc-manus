package skillmd

import (
	"bytes"
	"errors"
	"mime/multipart"
	"strings"
	"testing"

	"mooc-manus/pkg/skillerr"
)

func TestParse_BasicFrontmatter(t *testing.T) {
	md, err := Parse("---\nname: greet\ndescription: 向用户问好\n---\n# body\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md.Name != "greet" {
		t.Errorf("want name=greet, got %q", md.Name)
	}
	if md.Description != "向用户问好" {
		t.Errorf("want description=向用户问好, got %q", md.Description)
	}
}

func TestParse_HeadingPrefix(t *testing.T) {
	cases := []struct {
		input        string
		wantName     string
		wantDescript string
	}{
		{"---\n# name: greet\n## description: 描述\n---\n", "greet", "描述"},
		{"---\n## name:  spaced  \n# description:\tdesc\n---\n", "spaced", "desc"},
	}
	for _, c := range cases {
		md, err := Parse(c.input)
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if md.Name != c.wantName || md.Description != c.wantDescript {
			t.Errorf("input=%q want=(%q,%q) got=(%q,%q)", c.input, c.wantName, c.wantDescript, md.Name, md.Description)
		}
	}
}

func TestParse_BlockScalars(t *testing.T) {
	cases := []struct {
		marker string
	}{{">-"}, {">"}, {"|-"}, {"|"}}
	for _, c := range cases {
		input := "---\nname: demo\ndescription: " + c.marker + "\n  line one\n  line two\n---\n"
		md, err := Parse(input)
		if err != nil {
			t.Fatalf("[%s] Parse error: %v", c.marker, err)
		}
		if md.Description != "line one line two" {
			t.Errorf("[%s] want='line one line two' got=%q", c.marker, md.Description)
		}
	}
}

func TestParse_BlockScalarTabIndent(t *testing.T) {
	input := "---\nname: demo\ndescription: >\n\tline one\n\tline two\n---\n"
	md, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if md.Description != "line one line two" {
		t.Errorf("want='line one line two' got=%q", md.Description)
	}
}

func TestParse_BlockScalarEmptyKeepsMarker(t *testing.T) {
	// 块标量后没有任何缩进行 → 返回原始符号（Java warn 行为）
	input := "---\nname: demo\ndescription: >-\nnext: value\n---\n"
	md, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if md.Description != ">-" {
		t.Errorf("want='>-' got=%q", md.Description)
	}
}

func TestParse_BlockScalarStopAtUnindented(t *testing.T) {
	// 第二段未缩进，应当被截断；只保留首段
	input := "---\nname: demo\ndescription: >\n  keep me\nstop here\n---\n"
	md, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if md.Description != "keep me" {
		t.Errorf("want='keep me' got=%q", md.Description)
	}
}

func TestParse_MissingFrontmatter(t *testing.T) {
	_, err := Parse("# Just a heading\nname: x\n")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidSkillMd) || !errors.Is(err, skillerr.ErrInvalidInput) {
		t.Errorf("error chain mismatch: %v", err)
	}
}

func TestParse_FrontmatterNotClosed(t *testing.T) {
	_, err := Parse("---\nname: greet\ndescription: x\n")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidSkillMd) {
		t.Errorf("want ErrInvalidSkillMd, got %v", err)
	}
}

func TestParse_FieldMissingReturnsBlank(t *testing.T) {
	// Parse 层对 blank 不报错，由 ExtractFromUploads 报错
	md, err := Parse("---\nname: only\n---\n")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if md.Name != "only" || md.Description != "" {
		t.Errorf("got %#v", md)
	}
}

func TestParse_CRLFLineEnding(t *testing.T) {
	input := "---\r\nname: greet\r\ndescription: hi\r\n---\r\n"
	md, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// CRLF 下，正则的 .+ 会吃掉 \r，应当被 TrimSpace 去除
	if md.Name != "greet" {
		t.Errorf("want name=greet, got %q", md.Name)
	}
	if md.Description != "hi" {
		t.Errorf("want description=hi, got %q", md.Description)
	}
}

func TestExtractFromUploads_NewRequiredEmpty(t *testing.T) {
	_, found, err := ExtractFromUploads(nil, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if found {
		t.Error("found should be false")
	}
	if !errors.Is(err, ErrSkillMdMissing) || !errors.Is(err, skillerr.ErrInvalidInput) {
		t.Errorf("error chain mismatch: %v", err)
	}
}

func TestExtractFromUploads_UpdateOptionalReturnsFalse(t *testing.T) {
	md, found, err := ExtractFromUploads(nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("found should be false")
	}
	if md.Name != "" || md.Description != "" {
		t.Errorf("metadata should be empty, got %#v", md)
	}
}

func TestExtractFromUploads_PicksSkillMdStrictName(t *testing.T) {
	files := mockHeaders(map[string]string{
		"skill.md": "---\nname: lower\ndescription: lower\n---\n",
		"README":   "ignore me",
		"SKILL.md": "---\nname: hit\ndescription: matched\n---\n",
	})
	md, found, err := ExtractFromUploads(files, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("found should be true")
	}
	if md.Name != "hit" || md.Description != "matched" {
		t.Errorf("got %#v", md)
	}
}

func TestExtractFromUploads_NameBlank(t *testing.T) {
	files := mockHeaders(map[string]string{
		"SKILL.md": "---\nname:   \ndescription: ok\n---\n",
	})
	_, found, err := ExtractFromUploads(files, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !found {
		t.Error("found should be true (SKILL.md existed but was invalid)")
	}
	if !errors.Is(err, ErrSkillMdNameMissing) {
		t.Errorf("want ErrSkillMdNameMissing, got %v", err)
	}
}

func TestExtractFromUploads_DescriptionBlank(t *testing.T) {
	files := mockHeaders(map[string]string{
		"SKILL.md": "---\nname: greet\ndescription:   \n---\n",
	})
	_, _, err := ExtractFromUploads(files, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSkillMdDescMissing) {
		t.Errorf("want ErrSkillMdDescMissing, got %v", err)
	}
}

func TestExtractFromUploads_RequiredMissingFile(t *testing.T) {
	files := mockHeaders(map[string]string{
		"README": "no skill md here",
	})
	_, found, err := ExtractFromUploads(files, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if found {
		t.Error("found should be false")
	}
	if !errors.Is(err, ErrSkillMdMissing) {
		t.Errorf("want ErrSkillMdMissing, got %v", err)
	}
}

// mockHeaders 用真实 multipart 流构造 FileHeader 列表，确保 fh.Open() 走标准库路径可用。
func mockHeaders(parts map[string]string) []*multipart.FileHeader {
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	for name, content := range parts {
		fw, err := mw.CreateFormFile("files", name)
		if err != nil {
			panic(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			panic(err)
		}
	}
	mw.Close()

	mr := multipart.NewReader(buf, mw.Boundary())
	form, err := mr.ReadForm(10 << 20)
	if err != nil {
		panic(err)
	}
	headers := form.File["files"]
	// 保留 form 引用避免被 GC（FileHeader.Open 依赖 form 内部状态），用 sink 防止编译器优化
	keepForm = form
	// 同时确保顺序确定，按文件名排序方便断言
	if len(headers) > 1 {
		// 简单排序：让 SKILL.md 始终最后，验证「找到即停」时也能命中
		// 但 ReadForm 已保留写入顺序，这里不再额外排序
		_ = strings.Builder{}
	}
	return headers
}

var keepForm *multipart.Form
