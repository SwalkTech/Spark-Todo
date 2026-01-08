//go:build windows
// +build windows

package main

import (
	"context"
	"syscall"

	"golang.org/x/sys/windows"
)

func showWaterReminderSystemCentered(_ context.Context, title, message string) error {
	titleUTF16, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return err
	}
	messageUTF16, err := syscall.UTF16PtrFromString(message)
	if err != nil {
		return err
	}

	// hwnd=0：无 owner，使弹窗居中于当前屏幕（与应用面板位置无关）。
	// MB_TOPMOST/MB_SETFOREGROUND：确保可见性，避免被其它窗口遮挡。
	windows.MessageBox(
		windows.HWND(0),
		messageUTF16,
		titleUTF16,
		windows.MB_OK|windows.MB_ICONINFORMATION|windows.MB_TOPMOST|windows.MB_SETFOREGROUND,
	)
	return nil
}
