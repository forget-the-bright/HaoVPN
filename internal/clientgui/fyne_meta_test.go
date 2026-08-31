package clientgui

import (
	"testing"

	"haovpn/internal/brand"
)

// TestFyneAppMetadataFyneDo 确保 Migrations.fyneDo 为 true（抑制 not migrated 警告）。
func TestFyneAppMetadataFyneDo(t *testing.T) {
	m := fyneAppMetadata()
	if m.ID != brand.GUIAppID {
		t.Fatalf("ID=%q want %q", m.ID, brand.GUIAppID)
	}
	if m.Name != brand.Name {
		t.Fatalf("Name=%q want %q", m.Name, brand.Name)
	}
	if !m.Migrations["fyneDo"] {
		t.Fatal("Migrations[fyneDo] 须为 true")
	}
}
