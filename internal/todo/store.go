package todo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	// modernc.org/sqlite 是纯 Go 的 SQLite 驱动，方便跨平台打包（无需 CGO）。
	sqlite "modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

// Store 封装本地 SQLite 的所有读写能力：
// - 打开 DB、执行 PRAGMA
// - 建表/迁移（尽量向后兼容）
// - 组/任务/设置 的 CRUD
//
// 该应用是单用户桌面工具，因此这里将连接池限制为单连接（SetMaxOpenConns(1)），
// 以降低 SQLite 锁/并发带来的复杂度，并配合 busy_timeout 做“温和等待”。
type Store struct {
	db *sql.DB
}

const (
	// 这些上限用 rune 数计数（而不是字节数），避免中文等多字节字符导致“看起来不长但字节很大”的体验问题。
	maxGroupNameRunes   = 50
	maxTaskTitleRunes   = 200
	maxTaskContentRunes = 1000
	maxViewModeRunes    = 20
)

// DefaultDBPath 返回默认数据库路径（并确保目录存在）。
//
// 选择用户级配置目录（UserConfigDir）而不是程序目录的原因：
// - Windows 下 Program Files 常无写权限
// - 用户数据与程序分离，升级/重装更安全
func DefaultDBPath(appName string) (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get user config dir: %w", err)
	}

	appDir := filepath.Join(cfgDir, appName)
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", fmt.Errorf("create app data dir: %w", err)
	}

	return filepath.Join(appDir, "todo.db"), nil
}

// Open 打开（或创建）SQLite 数据库并完成初始化：
// - applyPragmas：开启外键、WAL、busy_timeout
// - migrate：建表/补列/建索引
// - ensureDefaultSettings / ensureDefaultGroup：写入默认数据，避免“空配置/空分组”导致 UI 交互尴尬
func Open(dbPath string) (*Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, errors.New("db path is empty")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.applyPragmas(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := s.ensureDefaultSettings(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := s.ensureDefaultGroup(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

// Close 关闭底层数据库连接。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// applyPragmas 设置 SQLite 运行参数（每次打开后都设置，避免依赖 DSN 拼接的可移植性问题）。
//
// - foreign_keys：启用外键与级联删除
// - busy_timeout：避免“偶发锁冲突”直接报错，最多等待 5s
// - journal_mode=WAL：提升并发读写体验（尤其是频繁写入的小应用）
func (s *Store) applyPragmas(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("pragma foreign_keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("pragma busy_timeout: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		return fmt.Errorf("pragma journal_mode: %w", err)
	}
	return nil
}

// migrate 确保数据库 schema 符合当前版本需求。
//
// 这里采用“幂等迁移”策略：
// - 新表/索引：用 IF NOT EXISTS
// - 老版本缺列：通过 PRAGMA table_info + ALTER TABLE ADD COLUMN 补齐
func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL CHECK (status IN ('todo','doing','done')),
			important INTEGER NOT NULL DEFAULT 0 CHECK (important IN (0,1)),
			urgent INTEGER NOT NULL DEFAULT 0 CHECK (urgent IN (0,1)),
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_group_status ON tasks(group_id, status)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	if err := s.ensureTasksColumns(ctx); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_tasks_important_urgent ON tasks(important, urgent)`); err != nil {
		return fmt.Errorf("create tasks important/urgent index: %w", err)
	}

	return nil
}

// ensureTasksColumns 用于向后兼容老版本数据库：
// - 读取 tasks 表列信息
// - 若缺少 important/urgent 列则补齐
func (s *Store) ensureTasksColumns(ctx context.Context) error {
	cols := map[string]bool{}

	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(tasks)`)
	if err != nil {
		return fmt.Errorf("read tasks schema: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan tasks schema: %w", err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate tasks schema: %w", err)
	}

	if !cols["important"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN important INTEGER NOT NULL DEFAULT 0 CHECK (important IN (0,1))`); err != nil {
			return fmt.Errorf("add tasks.important: %w", err)
		}
	}
	if !cols["urgent"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN urgent INTEGER NOT NULL DEFAULT 0 CHECK (urgent IN (0,1))`); err != nil {
			return fmt.Errorf("add tasks.urgent: %w", err)
		}
	}

	return nil
}

// ensureDefaultGroup 确保至少存在一个分组（用于首次启动的默认体验）。
//
// UI 中任务必须归属某个组；如果完全没有组，前端会处于“无法新建任务”的状态。
func (s *Store) ensureDefaultGroup(ctx context.Context) error {
	const defaultName = "默认"

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM groups`).Scan(&count); err != nil {
		return fmt.Errorf("count groups: %w", err)
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO groups(name, created_at, updated_at) VALUES(?, ?, ?)`,
		defaultName, now, now,
	)
	if err != nil {
		return fmt.Errorf("create default group: %w", err)
	}
	return nil
}

// ensureDefaultSettings 写入默认设置（仅在 key 不存在时插入，不覆盖用户已有选择）。
func (s *Store) ensureDefaultSettings(ctx context.Context) error {
	// Defaults: floating always-on-top by default, show done by default.
	defaults := map[string]string{
		"alwaysOnTop": "1",
		"hideDone":    "0",
		"viewMode":    "cards",
		"conciseMode": "0",
	}

	for k, v := range defaults {
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO settings(key, value) VALUES(?, ?)`,
			k, v,
		); err != nil {
			return fmt.Errorf("init settings %q: %w", k, err)
		}
	}
	return nil
}

// ListGroups 返回所有分组，按 id 升序排列（稳定、便于前端展示）。
func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at, updated_at FROM groups ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}
	return out, nil
}

// UpsertGroup 新增或更新分组。
//
// 约定：
// - id==0 => 新增
// - id>0  => 更新指定 id 的名称
//
// 该表对 name 做了 UNIQUE 约束：出现重复时返回稳定的中文错误提示。
func (s *Store) UpsertGroup(ctx context.Context, id int64, name string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Group{}, errors.New("组名不能为空")
	}
	if utf8.RuneCountInString(name) > maxGroupNameRunes {
		return Group{}, fmt.Errorf("组名过长（最多 %d 字）", maxGroupNameRunes)
	}

	now := time.Now().UnixMilli()
	if id == 0 {
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO groups(name, created_at, updated_at) VALUES(?, ?, ?)`,
			name, now, now,
		)
		if err != nil {
			if sqliteIsConstraint(err, sqlitelib.SQLITE_CONSTRAINT_UNIQUE) {
				return Group{}, errors.New("组名已存在")
			}
			return Group{}, fmt.Errorf("create group: %w", err)
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return Group{}, fmt.Errorf("get new group id: %w", err)
		}
		return Group{ID: newID, Name: name, CreatedAt: now, UpdatedAt: now}, nil
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE groups SET name = ?, updated_at = ? WHERE id = ?`,
		name, now, id,
	)
	if err != nil {
		if sqliteIsConstraint(err, sqlitelib.SQLITE_CONSTRAINT_UNIQUE) {
			return Group{}, errors.New("组名已存在")
		}
		return Group{}, fmt.Errorf("update group: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Group{}, fmt.Errorf("update group rows affected: %w", err)
	}
	if affected == 0 {
		return Group{}, fmt.Errorf("组不存在（id=%d）", id)
	}

	var g Group
	if err := s.db.QueryRowContext(ctx,
		`SELECT id, name, created_at, updated_at FROM groups WHERE id = ?`,
		id,
	).Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return Group{}, fmt.Errorf("reload group: %w", err)
	}
	return g, nil
}

// DeleteGroup 删除分组。
//
// tasks 表通过外键 `REFERENCES groups(id) ON DELETE CASCADE` 绑定，
// 因此删除分组会自动级联删除该组下的任务。
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("无效的组ID")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete group rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("组不存在（id=%d）", id)
	}
	return nil
}

// ListTasks 返回任务列表，按 updated_at 倒序（最近修改的在前）。
//
// important/urgent 在库中以 0/1 保存，这里转换为 bool 方便前端使用。
func (s *Store) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, group_id, title, content, status, important, urgent, created_at, updated_at FROM tasks ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		var t Task
		var status string
		var importantInt int
		var urgentInt int
		if err := rows.Scan(&t.ID, &t.GroupID, &t.Title, &t.Content, &status, &importantInt, &urgentInt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		parsed, err := ParseStatus(status)
		if err != nil {
			return nil, fmt.Errorf("parse task status: %w", err)
		}
		t.Status = parsed
		t.Important = importantInt == 1
		t.Urgent = urgentInt == 1
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return out, nil
}

// UpsertTask 新增或更新任务，并返回落库后的完整任务对象。
//
// 这里做了“前置校验”，目的：
// - 给前端更明确的错误信息（中文、可控）
// - 避免依赖数据库层错误（不同平台/驱动可能文案不同）
func (s *Store) UpsertTask(ctx context.Context, req Task) (Task, error) {
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)

	if req.GroupID <= 0 {
		return Task{}, errors.New("请选择一个组")
	}
	ok, err := s.groupExists(ctx, req.GroupID)
	if err != nil {
		return Task{}, err
	}
	if !ok {
		return Task{}, fmt.Errorf("组不存在（id=%d）", req.GroupID)
	}
	if req.Title == "" {
		return Task{}, errors.New("任务标题不能为空")
	}
	if utf8.RuneCountInString(req.Title) > maxTaskTitleRunes {
		return Task{}, fmt.Errorf("任务标题过长（最多 %d 字）", maxTaskTitleRunes)
	}
	if utf8.RuneCountInString(req.Content) > maxTaskContentRunes {
		return Task{}, fmt.Errorf("任务内容过长（最多 %d 字）", maxTaskContentRunes)
	}
	if _, err := ParseStatus(string(req.Status)); err != nil {
		return Task{}, err
	}

	now := time.Now().UnixMilli()
	if req.ID == 0 {
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO tasks(group_id, title, content, status, important, urgent, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			req.GroupID, req.Title, req.Content, string(req.Status), boolTo01Int(req.Important), boolTo01Int(req.Urgent), now, now,
		)
		if err != nil {
			return Task{}, fmt.Errorf("create task: %w", err)
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return Task{}, fmt.Errorf("get new task id: %w", err)
		}
		req.ID = newID
		req.CreatedAt = now
		req.UpdatedAt = now
		return req, nil
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE tasks
		 SET group_id = ?, title = ?, content = ?, status = ?, important = ?, urgent = ?, updated_at = ?
		 WHERE id = ?`,
		req.GroupID, req.Title, req.Content, string(req.Status), boolTo01Int(req.Important), boolTo01Int(req.Urgent), now, req.ID,
	)
	if err != nil {
		return Task{}, fmt.Errorf("update task: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Task{}, fmt.Errorf("update task rows affected: %w", err)
	}
	if affected == 0 {
		return Task{}, fmt.Errorf("任务不存在（id=%d）", req.ID)
	}

	var t Task
	var status string
	var importantInt int
	var urgentInt int
	if err := s.db.QueryRowContext(ctx,
		`SELECT id, group_id, title, content, status, important, urgent, created_at, updated_at FROM tasks WHERE id = ?`,
		req.ID,
	).Scan(&t.ID, &t.GroupID, &t.Title, &t.Content, &status, &importantInt, &urgentInt, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return Task{}, fmt.Errorf("reload task: %w", err)
	}
	parsed, err := ParseStatus(status)
	if err != nil {
		return Task{}, fmt.Errorf("parse task status: %w", err)
	}
	t.Status = parsed
	t.Important = importantInt == 1
	t.Urgent = urgentInt == 1
	return t, nil
}

// DeleteTask 删除任务。
func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("无效的任务ID")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete task rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("任务不存在（id=%d）", id)
	}
	return nil
}

// GetSettings 读取所有设置键值并返回 Settings 结构。
//
// 设计为"有默认值 + 部分覆盖"：
// - 任何缺失的 key 会回落到默认值
// - 多余的 key 被忽略，方便未来扩展
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	settings := Settings{
		AlwaysOnTop: true,
		HideDone:    false,
		ViewMode:    "cards",
		ConciseMode: false,
	}

	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return Settings{}, fmt.Errorf("list settings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Settings{}, fmt.Errorf("scan settings: %w", err)
		}
		switch key {
		case "alwaysOnTop":
			settings.AlwaysOnTop = value == "1" || strings.EqualFold(value, "true")
		case "hideDone":
			settings.HideDone = value == "1" || strings.EqualFold(value, "true")
		case "viewMode":
			settings.ViewMode = normalizeViewMode(value)
		case "conciseMode":
			settings.ConciseMode = value == "1" || strings.EqualFold(value, "true")
		}
	}
	if err := rows.Err(); err != nil {
		return Settings{}, fmt.Errorf("iterate settings: %w", err)
	}

	return settings, nil
}

// SetSettings 将 Settings 写回 settings 表（每个 key 单独 upsert）。
func (s *Store) SetSettings(ctx context.Context, settings Settings) error {
	if err := s.setSetting(ctx, "alwaysOnTop", boolTo01(settings.AlwaysOnTop)); err != nil {
		return err
	}
	if err := s.setSetting(ctx, "hideDone", boolTo01(settings.HideDone)); err != nil {
		return err
	}
	if err := s.setSetting(ctx, "viewMode", normalizeViewMode(settings.ViewMode)); err != nil {
		return err
	}
	if err := s.setSetting(ctx, "conciseMode", boolTo01(settings.ConciseMode)); err != nil {
		return err
	}
	return nil
}

// setSetting 对单个 key 做 upsert（INSERT ... ON CONFLICT DO UPDATE）。
func (s *Store) setSetting(ctx context.Context, key string, value string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	); err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}

// GetLastWaterReminderAt 返回上一次“喝水提醒”时间（UnixMilli）。
//
// 若从未记录过，则返回 0。
func (s *Store) GetLastWaterReminderAt(ctx context.Context) (int64, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, "lastWaterReminderAt").Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get lastWaterReminderAt: %w", err)
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	ts, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse lastWaterReminderAt: %w", err)
	}
	if ts <= 0 {
		return 0, nil
	}
	return ts, nil
}

// SetLastWaterReminderAt 保存“喝水提醒”时间（UnixMilli）。
func (s *Store) SetLastWaterReminderAt(ctx context.Context, unixMilli int64) error {
	if unixMilli <= 0 {
		unixMilli = 0
	}
	return s.setSetting(ctx, "lastWaterReminderAt", strconv.FormatInt(unixMilli, 10))
}

// boolTo01 将 bool 编码为 "0"/"1"（便于与 SQLite 的 TEXT 设置表统一）。
func boolTo01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// groupExists 检查分组是否存在，用于在写入任务前给出更友好的错误。
func (s *Store) groupExists(ctx context.Context, groupID int64) (bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM groups WHERE id = ?`, groupID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check group exists: %w", err)
	}
	return true, nil
}

// sqliteIsConstraint 判断错误是否为 SQLite 的特定约束错误码（例如 UNIQUE）。
func sqliteIsConstraint(err error, code int) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code() == code
	}
	return false
}

// boolTo01Int 将 bool 编码为 0/1（用于 tasks 表的整数列）。
func boolTo01Int(b bool) int {
	if b {
		return 1
	}
	return 0
}

// normalizeViewMode 将 viewMode 规范化为受支持的值（"list" 或 "cards"），其它输入回退到 "cards"。
func normalizeViewMode(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if utf8.RuneCountInString(v) > maxViewModeRunes {
		return "cards"
	}
	switch v {
	case "list", "cards":
		return v
	default:
		return "cards"
	}
}
