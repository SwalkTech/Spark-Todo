package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

// 当前版本信息（每次发布前需要手动更新）
const (
	Version = "1.1.0"
	Name    = "Spark-Todo"
)

// ReleaseInfo 表示一个发布版本的信息
type ReleaseInfo struct {
	Version     string `json:"version"`     // 版本号，如 "1.1.0"
	Name        string `json:"name"`        // 版本名称，如 "v1.1.0 - 简洁模式更新"
	Description string `json:"description"` // 版本描述/更新内容
	PublishedAt string `json:"publishedAt"` // 发布时间
	DownloadURL string `json:"downloadUrl"` // 下载链接（exe 或安装包）
	PageURL     string `json:"pageUrl"`     // Release 页面链接
	Required    bool   `json:"required"`    // 是否强制更新
}

// UpdateCheckResult 表示更新检查结果
type UpdateCheckResult struct {
	HasUpdate      bool         `json:"hasUpdate"`      // 是否有更新
	CurrentVersion string       `json:"currentVersion"` // 当前版本
	LatestRelease  *ReleaseInfo `json:"latestRelease"`  // 最新版本信息（如果有更新）
}

// UpdateChecker 负责检查更新
type UpdateChecker struct {
	// UpdateURL 是检查更新的 URL
	// 可以是 GitHub Releases API 或自定义服务器
	UpdateURL string
	// Timeout 是 HTTP 请求超时时间
	Timeout time.Duration
}

// NewUpdateChecker 创建更新检查器
func NewUpdateChecker(updateURL string) *UpdateChecker {
	if updateURL == "" {
		// 默认使用 GitHub Releases API
		// 实际部署时替换为你的 GitHub 仓库
		updateURL = "https://api.github.com/repos/yourusername/Spark-Todo/releases/latest"
	}
	return &UpdateChecker{
		UpdateURL: updateURL,
		Timeout:   10 * time.Second,
	}
}

// CheckUpdate 检查是否有新版本
func (uc *UpdateChecker) CheckUpdate(ctx context.Context) (*UpdateCheckResult, error) {
	result := &UpdateCheckResult{
		CurrentVersion: Version,
		HasUpdate:      false,
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, "GET", uc.UpdateURL, nil)
	if err != nil {
		return result, fmt.Errorf("create request: %w", err)
	}

	// 设置 User-Agent
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s (%s)", Name, Version, runtime.GOOS))

	// 发送请求
	client := &http.Client{Timeout: uc.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("fetch update info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("update server returned status %d", resp.StatusCode)
	}

	// 解析 GitHub Release 响应
	var githubRelease struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&githubRelease); err != nil {
		return result, fmt.Errorf("parse response: %w", err)
	}

	// 提取版本号（去掉 v 前缀）
	latestVersion := githubRelease.TagName
	if len(latestVersion) > 0 && latestVersion[0] == 'v' {
		latestVersion = latestVersion[1:]
	}

	// 比较版本
	if compareVersion(latestVersion, Version) > 0 {
		result.HasUpdate = true

		// 查找合适的下载链接
		downloadURL := ""
		for _, asset := range githubRelease.Assets {
			// 优先选择安装包，其次选择 exe
			if runtime.GOOS == "windows" {
				if len(downloadURL) == 0 || isInstallerAsset(asset.Name) {
					downloadURL = asset.BrowserDownloadURL
				}
			}
		}

		result.LatestRelease = &ReleaseInfo{
			Version:     latestVersion,
			Name:        githubRelease.Name,
			Description: githubRelease.Body,
			PublishedAt: githubRelease.PublishedAt,
			DownloadURL: downloadURL,
			PageURL:     githubRelease.HTMLURL,
			Required:    false, // 可以根据版本号规则判断是否强制更新
		}
	}

	return result, nil
}

// isInstallerAsset 判断是否为安装包
func isInstallerAsset(name string) bool {
	return len(name) > 13 && name[len(name)-13:] == "-installer.exe"
}

// compareVersion 比较两个版本号
// 返回值：1 表示 v1 > v2，-1 表示 v1 < v2，0 表示相等
func compareVersion(v1, v2 string) int {
	// 简化版本比较，支持 x.y.z 格式
	var major1, minor1, patch1 int
	var major2, minor2, patch2 int

	fmt.Sscanf(v1, "%d.%d.%d", &major1, &minor1, &patch1)
	fmt.Sscanf(v2, "%d.%d.%d", &major2, &minor2, &patch2)

	if major1 != major2 {
		if major1 > major2 {
			return 1
		}
		return -1
	}
	if minor1 != minor2 {
		if minor1 > minor2 {
			return 1
		}
		return -1
	}
	if patch1 != patch2 {
		if patch1 > patch2 {
			return 1
		}
		return -1
	}
	return 0
}
