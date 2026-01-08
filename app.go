package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"time"

	"spark-todo/internal/todo"
	"spark-todo/internal/version"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 后端入口对象。
//
// 前端通过 `frontend/wailsjs/go/main/App.*` 中的桥接代码调用本结构体的导出方法：
//   - 任务/组的增删改查
//   - 设置项（置顶、隐藏已完成、视图模式）的读写
//
// 注意：前端拿到的是序列化后的数据（JSON），字段通过 internal/todo 下的模型定义对齐。
type App struct {
	// ctx 为 Wails 在 startup 时注入的上下文：
	// - 用于调用 runtime API（例如 WindowSetAlwaysOnTop）
	// - 也作为数据库操作的 context 传递（便于未来做取消/超时）
	ctx context.Context

	// store 封装了 SQLite 读写与迁移逻辑。
	store *todo.Store

	// startupErr 记录启动阶段失败原因（如无法确定 DB 路径、打开 DB 失败等），
	// 供后续 API 调用时返回更友好的错误信息。
	startupErr error

	// waterReminderShowing 用于防止"喝水提醒"弹窗重复叠加。
	//（例如用户未关闭弹窗时定时器再次触发，或多次前端初始化导致的重复调用）
	waterReminderShowing atomic.Bool

	// updateChecker 用于检查应用更新
	updateChecker *version.UpdateChecker
}

// NewApp 创建 App 实例。
//
// 实际初始化（打开数据库、读取设置）在 startup 回调中完成，因为只有那里能拿到 Wails runtime ctx。
func NewApp() *App {
	return &App{
		updateChecker: version.NewUpdateChecker(""),
	}
}

// startup 在应用启动时被 Wails 调用。
//
// 这里做三件事：
//  1. 保存 ctx，供后续调用 runtime API 与 DB 操作使用
//  2. 解析并打开默认数据库（必要时自动创建目录/建表/迁移）
//  3. 读取持久化设置，并应用到窗口（例如置顶）
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	dbPath, err := todo.DefaultDBPath("Spark-Todo")
	if err != nil {
		runtime.LogErrorf(ctx, "failed to resolve db path: %v", err)
		a.startupErr = fmt.Errorf("初始化失败：无法确定数据库路径：%w", err)
		return
	}

	s, err := todo.Open(dbPath)
	if err != nil {
		runtime.LogErrorf(ctx, "failed to open db: %v", err)
		a.startupErr = fmt.Errorf("初始化失败：无法打开数据库：%w", err)
		return
	}
	a.store = s
	a.startupErr = nil

	settings, err := a.store.GetSettings(ctx)
	if err == nil {
		runtime.WindowSetAlwaysOnTop(ctx, settings.AlwaysOnTop)
	}
}

// shutdown 在应用退出时被 Wails 调用，用于释放资源。
func (a *App) shutdown(ctx context.Context) {
	_ = ctx
	if a.store != nil {
		_ = a.store.Close()
	}
}

// ensureStoreReady 是所有对外 API 的统一前置检查：
// - store 已就绪：允许继续
// - startup 曾失败：返回启动阶段错误，让前端能提示更明确的原因
// - 启动仍未完成：返回“尚未初始化完成”的提示
func (a *App) ensureStoreReady() error {
	if a.store != nil {
		return nil
	}
	if a.startupErr != nil {
		return a.startupErr
	}
	return errors.New("应用尚未初始化完成")
}

// GetBoard 返回前端渲染所需的聚合数据：
// - groups：分组列表
// - tasks：任务列表
// - settings：用户设置
// - statuses：状态枚举（用于下拉选项/校验）
func (a *App) GetBoard() (todo.Board, error) {
	if err := a.ensureStoreReady(); err != nil {
		return todo.Board{}, err
	}

	groups, err := a.store.ListGroups(a.ctx)
	if err != nil {
		return todo.Board{}, err
	}
	tasks, err := a.store.ListTasks(a.ctx)
	if err != nil {
		return todo.Board{}, err
	}
	settings, err := a.store.GetSettings(a.ctx)
	if err != nil {
		return todo.Board{}, err
	}

	return todo.Board{
		Groups:   groups,
		Tasks:    tasks,
		Settings: settings,
		Statuses: []todo.Status{todo.StatusTodo, todo.StatusDoing, todo.StatusDone},
	}, nil
}

// UpsertGroup 新增或更新一个分组：
// - id==0 表示新增
// - id>0 表示按 ID 更新名称
func (a *App) UpsertGroup(id int64, name string) (todo.Group, error) {
	if err := a.ensureStoreReady(); err != nil {
		return todo.Group{}, err
	}
	return a.store.UpsertGroup(a.ctx, id, name)
}

// DeleteGroup 删除分组（以及外键级联删除其下任务）。
func (a *App) DeleteGroup(id int64) error {
	if err := a.ensureStoreReady(); err != nil {
		return err
	}
	return a.store.DeleteGroup(a.ctx, id)
}

// UpsertTask 新增或更新任务。
func (a *App) UpsertTask(task todo.Task) (todo.Task, error) {
	if err := a.ensureStoreReady(); err != nil {
		return todo.Task{}, err
	}
	return a.store.UpsertTask(a.ctx, task)
}

// DeleteTask 删除任务。
func (a *App) DeleteTask(id int64) error {
	if err := a.ensureStoreReady(); err != nil {
		return err
	}
	return a.store.DeleteTask(a.ctx, id)
}

// SetHideDone 更新“隐藏已完成”开关，并返回更新后的 Settings（便于前端就地更新 UI）。
func (a *App) SetHideDone(hide bool) (todo.Settings, error) {
	if err := a.ensureStoreReady(); err != nil {
		return todo.Settings{}, err
	}

	settings, err := a.store.GetSettings(a.ctx)
	if err != nil {
		return todo.Settings{}, err
	}
	settings.HideDone = hide
	if err := a.store.SetSettings(a.ctx, settings); err != nil {
		return todo.Settings{}, err
	}
	return settings, nil
}

// SetAlwaysOnTop 更新“置顶悬浮”开关：
// - 持久化到 settings 表
// - 立即调用 runtime.WindowSetAlwaysOnTop 让窗口生效
func (a *App) SetAlwaysOnTop(on bool) (todo.Settings, error) {
	if err := a.ensureStoreReady(); err != nil {
		return todo.Settings{}, err
	}

	settings, err := a.store.GetSettings(a.ctx)
	if err != nil {
		return todo.Settings{}, err
	}
	settings.AlwaysOnTop = on
	if err := a.store.SetSettings(a.ctx, settings); err != nil {
		return todo.Settings{}, err
	}
	runtime.WindowSetAlwaysOnTop(a.ctx, on)
	return settings, nil
}

// SetViewMode 更新视图模式（"list" 或 "cards"）。
func (a *App) SetViewMode(mode string) (todo.Settings, error) {
	if err := a.ensureStoreReady(); err != nil {
		return todo.Settings{}, err
	}

	settings, err := a.store.GetSettings(a.ctx)
	if err != nil {
		return todo.Settings{}, err
	}
	settings.ViewMode = mode
	if err := a.store.SetSettings(a.ctx, settings); err != nil {
		return todo.Settings{}, err
	}
	return settings, nil
}

// SetTheme 更新主题（"light" 或 "dark"）。
func (a *App) SetTheme(theme string) (todo.Settings, error) {
	if err := a.ensureStoreReady(); err != nil {
		return todo.Settings{}, err
	}

	settings, err := a.store.GetSettings(a.ctx)
	if err != nil {
		return todo.Settings{}, err
	}
	settings.Theme = theme
	if err := a.store.SetSettings(a.ctx, settings); err != nil {
		return todo.Settings{}, err
	}
	return settings, nil
}

// SetConciseMode 更新"简洁模式"开关：
// - 持久化到 settings 表
// - 简洁模式控制窗口是否显示边框（Frameless 属性）
// 注意：Wails 的 Frameless 属性在窗口创建时设置，运行时无法动态修改。
// 此方法仅保存设置，实际边框切换需要重启应用才能生效。
func (a *App) SetConciseMode(on bool) (todo.Settings, error) {
	if err := a.ensureStoreReady(); err != nil {
		return todo.Settings{}, err
	}

	settings, err := a.store.GetSettings(a.ctx)
	if err != nil {
		return todo.Settings{}, err
	}
	settings.ConciseMode = on
	if err := a.store.SetSettings(a.ctx, settings); err != nil {
		return todo.Settings{}, err
	}
	return settings, nil
}

// Quit 退出应用程序。
func (a *App) Quit() {
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// Restart 重启应用程序。
func (a *App) Restart() error {
	if a.ctx == nil {
		return errors.New("应用尚未初始化完成")
	}

	// 获取当前可执行文件路径
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	// 在后台启动新进程
	cmd := exec.Command(executable)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动新进程失败: %w", err)
	}

	// 延迟退出当前进程，给新进程一点启动时间
	go func() {
		time.Sleep(500 * time.Millisecond)
		runtime.Quit(a.ctx)
	}()

	return nil
}

// ShowWaterReminder 触发一次"喝水提醒"。
//
// 该提醒应出现在"电脑屏幕中间"，与 todoP1 面板位置无关，因此由后端调用系统级弹窗实现。
func (a *App) ShowWaterReminder() error {
	if a.ctx == nil {
		return errors.New("应用尚未初始化完成")
	}

	if !a.waterReminderShowing.CompareAndSwap(false, true) {
		return nil
	}
	defer a.waterReminderShowing.Store(false)

	// 记录“上一次提醒时间”，避免用户短时间内反复打开应用导致重复弹窗。
	// 规则：若距离上次提醒未满 1 小时，则本次不打扰。
	if a.store != nil {
		lastAt, err := a.store.GetLastWaterReminderAt(a.ctx)
		if err != nil {
			runtime.LogErrorf(a.ctx, "failed to read last water reminder time: %v", err)
		} else if lastAt > 0 && time.Since(time.UnixMilli(lastAt)) < time.Hour {
			return nil
		}
	}

	if err := showWaterReminderSystemCentered(a.ctx, "喝水提醒", "喝水小提醒：该喝水了"); err != nil {
		return err
	}

	if a.store != nil {
		if err := a.store.SetLastWaterReminderAt(a.ctx, time.Now().UnixMilli()); err != nil {
			// 持久化失败不影响本次提醒展示，避免前端降级为 Toast（会影响体验）。
			runtime.LogErrorf(a.ctx, "failed to persist last water reminder time: %v", err)
		}
	}

	return nil
}

// GetVersion 获取当前应用版本
func (a *App) GetVersion() string {
	return version.Version
}

// CheckUpdate 检查更新
func (a *App) CheckUpdate() (*version.UpdateCheckResult, error) {
	if a.ctx == nil {
		return nil, errors.New("应用尚未初始化完成")
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()

	result, err := a.updateChecker.CheckUpdate(ctx)
	if err != nil {
		return nil, fmt.Errorf("检查更新失败: %w", err)
	}

	return result, nil
}

// OpenURL 在浏览器中打开 URL
func (a *App) OpenURL(url string) error {
	if a.ctx == nil {
		return errors.New("应用尚未初始化完成")
	}

	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}
