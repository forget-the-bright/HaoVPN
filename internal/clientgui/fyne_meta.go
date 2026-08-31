package clientgui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"haovpn/internal/brand"
)

// fyneAppMetadata 组装 Fyne 应用元数据（含 fyneDo 迁移开关）。
// 纯 go build 不读 FyneApp.toml，须在 NewWithID 前 SetMetadata，并配合 -tags migrated_fynedo。
func fyneAppMetadata() fyne.AppMetadata {
	return fyne.AppMetadata{
		ID:      brand.GUIAppID,
		Name:    brand.Name,
		Version: "", // 版本由构建 ldflags / VERSION 同步，不在此硬编码
		Migrations: map[string]bool{
			"fyneDo": true,
		},
	}
}

// applyFyneDoMigration 在创建 App 前写入元数据，抑制「not migrated」警告。
func applyFyneDoMigration() {
	app.SetMetadata(fyneAppMetadata())
}
