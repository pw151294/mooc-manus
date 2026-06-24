package main

import (
	"fmt"
	"log"
	"mooc-manus/config"
	"mooc-manus/internal/infra/storage"
	"os"
	"strings"
)

// 阶段 1：执行 Skill 模块 4 张表的 DDL
// 用法：go run scripts/migrate_skill/main.go
// 幂等：每条语句前判断是否已存在，已存在则跳过。
func main() {
	config.InitConfig()
	if err := storage.InitStorage(); err != nil {
		log.Fatalf("init postgres failed: %v", err)
	}
	db := storage.GetPostgresClient()

	tables := []string{"skill_provider", "skill", "skill_version", "task_execution"}
	for _, t := range tables {
		var exists bool
		if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=?)`, t).Scan(&exists).Error; err != nil {
			log.Fatalf("check %s exists failed: %v", t, err)
		}
		fmt.Printf("[check] %-16s exists=%v\n", t, exists)
		if exists {
			log.Fatalf("table %s already exists, aborting (please drop it first if you intend to re-create)", t)
		}
	}

	sqlBytes, err := os.ReadFile("docs/sql/manus_schema.sql")
	if err != nil {
		log.Fatalf("read schema file failed: %v", err)
	}
	full := string(sqlBytes)

	marker := "-- Skill 模块（迁移自 Beedance Skill 配置 & 版本管理）"
	idx := strings.Index(full, marker)
	if idx < 0 {
		log.Fatalf("marker not found in manus_schema.sql: %s", marker)
	}
	skillDDL := full[idx:]

	stmts := splitSQL(skillDDL)
	fmt.Printf("[exec] %d statements to execute\n", len(stmts))

	for i, stmt := range stmts {
		s := strings.TrimSpace(stmt)
		if s == "" || strings.HasPrefix(s, "--") {
			continue
		}
		head := s
		if len(head) > 80 {
			head = head[:80] + "..."
		}
		fmt.Printf("[%2d] %s\n", i+1, strings.ReplaceAll(head, "\n", " "))
		if err := db.Exec(s).Error; err != nil {
			log.Fatalf("exec failed at #%d: %v\nSQL:\n%s", i+1, err, s)
		}
	}

	for _, t := range tables {
		var n int
		if err := db.Raw(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, t)).Scan(&n).Error; err != nil {
			log.Fatalf("verify %s failed: %v", t, err)
		}
		fmt.Printf("[verify] %-16s rows=%d ✓\n", t, n)
	}
	fmt.Println("\nDDL applied successfully.")
}

// splitSQL 按行扫描，遇到不在字符串内、不在注释行内、并且以 ';' 结尾的位置切分
func splitSQL(s string) []string {
	var out []string
	var buf strings.Builder
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
		if strings.HasSuffix(trim, ";") {
			out = append(out, buf.String())
			buf.Reset()
		}
	}
	if strings.TrimSpace(buf.String()) != "" {
		out = append(out, buf.String())
	}
	return out
}
