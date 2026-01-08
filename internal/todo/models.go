package todo

import "fmt"

// Status 表示任务状态。
//
// 为了与前端（JS/TS）对齐，这里使用 string 枚举值，并在数据库层通过 CHECK 约束保证合法性。
type Status string

const (
	// StatusTodo 表示“待办”。
	StatusTodo Status = "todo"
	// StatusDoing 表示“进行中”。
	StatusDoing Status = "doing"
	// StatusDone 表示“已完成”。
	StatusDone Status = "done"
)

// ParseStatus 将字符串解析为 Status，并校验是否在允许集合内。
//
// 该校验用于：
// - 读库：把文本状态转为强类型
// - 写库：在落库前尽早发现无效输入（避免依赖数据库错误）
func ParseStatus(s string) (Status, error) {
	switch Status(s) {
	case StatusTodo, StatusDoing, StatusDone:
		return Status(s), nil
	default:
		return "", fmt.Errorf("无效的任务状态: %q", s)
	}
}

// Group 表示任务分组。
//
// 时间字段使用 UnixMilli（毫秒时间戳）：
// - JSON/JS 侧可以用 Number 承载
// - 便于按更新时间排序
type Group struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Task 表示一个任务条目。
//
// Important/Urgent 用于在前端四象限中定位任务：
// - important && urgent   => 重要且紧急
// - important && !urgent  => 重要不紧急
// - !important && urgent  => 不重要但紧急
// - !important && !urgent => 不重要不紧急
type Task struct {
	ID        int64  `json:"id"`
	GroupID   int64  `json:"groupId"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Status    Status `json:"status"`
	Important bool   `json:"important"`
	Urgent    bool   `json:"urgent"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Settings 为用户偏好设置（持久化到 SQLite settings 表）。
type Settings struct {
	HideDone    bool   `json:"hideDone"`
	AlwaysOnTop bool   `json:"alwaysOnTop"`
	ViewMode    string `json:"viewMode"`    // "list" | "cards"
	ConciseMode bool   `json:"conciseMode"` // 简洁模式（控制窗口边框）
	Theme       string `json:"theme"`       // "light" | "dark"
}

// Board 是前端渲染所需的聚合数据（一次请求拿到全部视图需要的数据）。
type Board struct {
	Groups   []Group  `json:"groups"`
	Tasks    []Task   `json:"tasks"`
	Settings Settings `json:"settings"`
	Statuses []Status `json:"statuses"`
}
