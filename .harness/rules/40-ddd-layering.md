---
rule_id: R-40-ddd
severity: high
---

# DDD 三层职责与依赖方向

mooc-manus 严格遵循 Handler → Application → Domain → Repository 四层（DDD），上层依赖下层，下层不得反向依赖。本规则锁定层间边界与三态模型（PO / DO / DTO）的转换路径。

## 禁止行为

1. **禁止 interfaces 直接 import infrastructure**
   - `api/handlers/` 与 `api/routers/` 仅依赖 `internal/applications/`；DI 容器（`route.go::InitRouter`）除外
   - 违例：`api/handlers/skill.go` 直接 `import "mooc-manus/internal/infra/repositories"`

2. **禁止 domains 依赖 infrastructure 实现**
   - `internal/domains/` 只依赖 Repository 接口，不得 `import "mooc-manus/internal/infra/repositories"` 的具体实现包
   - Repository 接口在 `internal/domains/services/<domain>/repository.go` 或同等位置声明，实现位于 `internal/infra/repositories/`

3. **禁止 PO 暴露给 domains 之外**
   - `internal/infra/models` 中的 `XxxPO` 仅 Repository 内部使用
   - Repository 入参 / 返回值仅接受 PO；Domain Service 内部使用 DO；Handler / Application 仅接触 DTO

4. **禁止跨层调用**
   - Handler → Domain Service / Repository
   - Application Service → Repository / GORM
   - 反向依赖（Domain → Application）

## 要求行为

1. **三态模型转换链路（参考 `.harness/AGENTS.md` §五）**
   ```
   HTTP Request (JSON)
       ↓ c.ShouldBindJSON
   [DTO] XxxRequest
       ↓ ConvertXxxRequest2DO       (位于 internal/applications/dtos/xxx.go)
   [DO] XxxDO
       ↓ ConvertXxxDO2PO            (位于 internal/domains/models/xxx.go)
   [PO] XxxPO
       ↓ Repository.Create
   数据库
   ```

2. **跨业务域协作走 Repository 接口**
   - Agent Domain Service 通过 `SkillRepository` 接口读取 Skill 数据，不得直接调用 `SkillDomainService`
   - 依赖方向单向（Agent → Skill），禁止循环依赖

3. **DI 集中在 `api/routers/route.go::InitRouter`**
   - 严格按 Repository → Domain → Application → Handler 顺序构造
   - 同层内被依赖方在前

## Agent 行为

- 检测到"在 handler 内 import repositories"、"在 domain 内 import gorm 或 infra/models"、"PO 字段出现在 DTO" → 拒绝并要求按四层重构
- 跨域调用请求 → 引导添加 Repository 接口而非直接复用对方 Service
- 新增模块 → 优先参考 `internal/domains/services/tools/` 与 `internal/domains/services/skills/` 的现有范式

## 可验证性

- `ddd-layer-checker` 子代理：扫描 import 路径，违例标记 blocker
- 静态检查（grep）：
  - `grep -rn "internal/infra/repositories" internal/domains/` 应为空（除接口装配的 DI 入口）
  - `grep -rn "internal/infra/repositories" api/handlers/` 应为空
  - `grep -rn "gorm.io/gorm" internal/domains/` 应为空
- 单元测试：Repository mock 接口可在 Domain Service 测试中替换，验证依赖反转
