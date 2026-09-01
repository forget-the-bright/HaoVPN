package clientgui

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"haovpn/internal/brand"
	"haovpn/internal/clientapp"
)

// skipFatalDialog 单测子进程用：跳过 Fyne 对话框直接退出。
func skipFatalDialog(title, message string, err error) bool {
	if os.Getenv("HAOVPN_GUI_SKIP_DIALOG") != "1" {
		return false
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	} else if message != "" {
		fmt.Fprintln(os.Stderr, message)
	} else if title != "" {
		fmt.Fprintln(os.Stderr, title)
	}
	os.Exit(0)
	return true
}

// ShowAlreadyRunning 单实例冲突：自定义提示小窗，点确定后退出进程。
func ShowAlreadyRunning(message string) {
	if message == "" {
		message = clientapp.SingleInstanceUserMessage(false)
	}
	ShowFatalNotice("无法启动", message)
}

// ShowFatalNotice 展示阻塞式提示窗，用户确认后关闭进程。
//
// 环境变量 HAOVPN_GUI_SKIP_DIALOG=1 时跳过 UI，写 stderr 后 os.Exit(0)（供单测子进程）。
func ShowFatalNotice(title, message string) {
	if skipFatalDialog(title, message, nil) {
		return
	}
	a := newNoticeApp()
	runNoticeWindow(a, title, message)
	os.Exit(0)
}

// showFatalErrorOnApp 在已有 Fyne App 上展示致命错误（供 showLogin 早期失败）。
func showFatalErrorOnApp(a fyne.App, err error) {
	if err == nil {
		return
	}
	runNoticeWindow(a, "无法启动", err.Error())
}

func newNoticeApp() fyne.App {
	a := app.NewWithID(brand.GUIAppID)
	if AppTheme != nil {
		a.Settings().SetTheme(AppTheme)
	}
	return a
}

// runNoticeWindow 单窗口提示（无 dialog 套娃）；窗口标题固定为品牌名，headline 为内容区标题。
func runNoticeWindow(a fyne.App, headline, body string) {
	w := a.NewWindow(brand.Name)
	w.Resize(fyne.NewSize(420, 180))
	w.CenterOnScreen()
	w.SetFixedSize(true)

	done := func() {
		w.Close()
		a.Quit()
	}
	w.SetCloseIntercept(done)

	headLbl := widget.NewLabelWithStyle(headline, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	bodyLbl := widget.NewLabel(body)
	bodyLbl.Wrapping = fyne.TextWrapWord
	okBtn := widget.NewButton("确定", done)
	okBtn.Importance = widget.HighImportance

	content := container.NewVBox(
		layout.NewSpacer(),
		headLbl,
		bodyLbl,
		layout.NewSpacer(),
		container.NewCenter(okBtn),
	)
	w.SetContent(container.NewPadded(content))
	w.Show()
	a.Run()
	os.Exit(0)
}
