//go:build !windows

package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func showWaterReminderSystemCentered(ctx context.Context, title, message string) error {
	_, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.InfoDialog,
		Title:   title,
		Message: message,
	})
	return err
}
