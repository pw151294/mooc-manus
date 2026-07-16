package repositories

import (
	"context"
	"mooc-manus/internal/domains/models/evaluation"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 简化的测试用 PO，避免 jsonb 字段
type testInstancePO struct {
	ID                    string `gorm:"type:uuid;primaryKey"`
	TaskID                string `gorm:"type:uuid"`
	CaseID                string `gorm:"type:uuid"`
	AgentConfigSnapshotID string `gorm:"type:uuid"`
	Status                string `gorm:"type:varchar(24)"`
	Attempt               int
	ConversationID        string `gorm:"type:varchar(64)"`
	MessageID             string `gorm:"type:varchar(64)"`
	TraceID               string `gorm:"type:varchar(64)"`
	QueuedAt              *time.Time
	StartedAt             *time.Time
	FinishedAt            *time.Time
	HeartbeatAt           *time.Time
	DeadlineAt            *time.Time
	WorkerID              string `gorm:"type:varchar(64)"`
	ErrorMessage          string `gorm:"type:text"`
}

func (testInstancePO) TableName() string {
	return "eval_run_instance"
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// 手动创建表
	err = db.Exec(`
		CREATE TABLE IF NOT EXISTS eval_run_instance (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			case_id TEXT,
			agent_config_snapshot_id TEXT,
			status TEXT,
			attempt INTEGER,
			conversation_id TEXT,
			message_id TEXT,
			trace_id TEXT,
			queued_at DATETIME,
			started_at DATETIME,
			finished_at DATETIME,
			heartbeat_at DATETIME,
			deadline_at DATETIME,
			worker_id TEXT,
			error_message TEXT
		)
	`).Error
	require.NoError(t, err)

	return db
}

func TestCASStatusRacesOnce(t *testing.T) {
	db := setupTestDB(t)
	repo := NewEvalRunInstanceRepository(db)

	// 直接插入简化的测试数据
	instID := uuid.New().String()
	po := &testInstancePO{
		ID:                    instID,
		TaskID:                uuid.New().String(),
		CaseID:                uuid.New().String(),
		AgentConfigSnapshotID: uuid.New().String(),
		Status:                string(evaluation.InstanceStatusRunning),
		Attempt:               1,
		ConversationID:        "test-conv",
		MessageID:             "test-msg",
	}
	err := db.Create(po).Error
	require.NoError(t, err)

	// 两个 goroutine 同时尝试 CAS RUNNING → VERIFYING
	var wg sync.WaitGroup
	results := make([]bool, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.CASStatus(context.Background(), instID,
				evaluation.InstanceStatusRunning, evaluation.InstanceStatusVerifying)
			require.NoError(t, err)
			results[i] = ok
		}()
	}
	wg.Wait()

	// 断言：只应有一方 CAS 成功
	won := 0
	for _, r := range results {
		if r {
			won++
		}
	}
	assert.Equal(t, 1, won, "只应有一方 CAS 成功")

	// 验证最终状态为 VERIFYING
	var finalPO testInstancePO
	err = db.Where("id = ?", instID).First(&finalPO).Error
	require.NoError(t, err)
	assert.Equal(t, string(evaluation.InstanceStatusVerifying), finalPO.Status)
}
