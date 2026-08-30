package clientgui

import (
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"haovpn/internal/autostart"
	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/singleinstance"
)

// ServiceTakeoverAction 服务占用协调口时用户选择。
type ServiceTakeoverAction int

const (
	// ServiceTakeoverKeep 保持服务运行，关闭本次 GUI。
	ServiceTakeoverKeep ServiceTakeoverAction = iota
	// ServiceTakeoverTake 停止服务并由本 GUI 接管。
	ServiceTakeoverTake
)

// AskServiceTakeover 阻塞对话框：服务已在运行时询问是否接管。
func AskServiceTakeover() ServiceTakeoverAction {
	if os.Getenv("HAOVPN_GUI_SKIP_DIALOG") == "1" {
		return ServiceTakeoverKeep
	}
	result := ServiceTakeoverKeep
	a := newNoticeApp()
	w := a.NewWindow(brand.Name)
	w.Resize(fyne.NewSize(460, 220))
	w.CenterOnScreen()
	w.SetFixedSize(true)

	finish := func(act ServiceTakeoverAction) {
		result = act
		w.Close()
		a.Quit()
	}
	w.SetCloseIntercept(func() { finish(ServiceTakeoverKeep) })

	head := widget.NewLabelWithStyle("后台服务已在运行", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	body := widget.NewLabel("VPN 由 Windows 服务托管（无托盘）。可停止服务并由本程序接管，或保持服务并关闭本窗口。")
	body.Wrapping = fyne.TextWrapWord
	takeBtn := widget.NewButton("停止服务并接管", func() { finish(ServiceTakeoverTake) })
	takeBtn.Importance = widget.HighImportance
	keepBtn := widget.NewButton("保持服务，关闭窗口", func() { finish(ServiceTakeoverKeep) })

	w.SetContent(container.NewPadded(container.NewVBox(
		layout.NewSpacer(), head, body, layout.NewSpacer(),
		container.NewHBox(layout.NewSpacer(), keepBtn, takeBtn, layout.NewSpacer()),
	)))
	w.Show()
	a.Run()
	return result
}

// WaitSingleInstanceFree 轮询至协调口空闲或超时。
func WaitSingleInstanceFree(timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !singleinstance.ClientAlreadyRunning() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return !singleinstance.ClientAlreadyRunning()
}

// StopServiceForTakeover 停止 HaoVPN 客户端服务并等待（超时见 autostart.DefaultServiceStopTimeout）。
func StopServiceForTakeover() error {
	logger.Info("gui_takeover stop_service begin")
	err := autostart.ServiceStopAndWait(autostart.DefaultServiceStopTimeout)
	if err != nil {
		logger.Warn("gui_takeover stop_service: %v", err)
		return err
	}
	logger.Info("gui_takeover stop_service done")
	return nil
}

// resolveGUIExe 当前进程 exe 绝对路径。
func resolveGUIExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return absPath(exe)
}
