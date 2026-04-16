package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConfigModel_Navigation(t *testing.T) {
	// Initialize with default items
	m := newConfigModel()

	// 1. Move Down: j key
	m.Update(keyMsg('j'))
	if m.cursor != 1 {
		t.Errorf("Expected cursor at 1 after 'j', got %d", m.cursor)
	}

	// 2. Toggle Boolean: Auto-Update Checks is at items[1]
	// Current value is "true", space should toggle to "false"
	m.Update(specialKeyMsg(tea.KeySpace))
	if m.items[1].Value != "false" {
		t.Errorf("Expected toggled boolean value 'false', got %s", m.items[1].Value)
	}

	// 3. Enter Edit Mode: Max Complexity is at items[3] (non-option)
	m.cursor = 3
	m.Update(specialKeyMsg(tea.KeyEnter))
	if m.mode != configEditing {
		t.Errorf("Expected mode configEditing, got %v", m.mode)
	}

	// 4. Typing in Edit Mode: Append '0' to "15" -> "150"
	m.Update(keyMsg('0'))
	if m.editBuffer != "150" {
		t.Errorf("Expected editBuffer '150', got %q", m.editBuffer)
	}
}

func TestConfigModel_View(t *testing.T) {
	m := newConfigModel()
	m.width, m.height = 100, 40

	// Test Case 1: Standard Render
	// Assert presence of category headers and specific setting keys
	view := m.render()
	
	expectedStrings := []string{
		"Settings",        // Title
		"General",         // Category divider
		"Output Format",   // Item key
		"Quality Gate",    // Another category
	}

	for _, s := range expectedStrings {
		if !strings.Contains(view, s) {
			t.Errorf("Standard view missing expected string %q", s)
		}
	}

	// Test Case 2: Edit Mode Render
	m.cursor = 3 // Max Complexity
	m.Update(specialKeyMsg(tea.KeyEnter))
	
	editView := m.render()
	// Assert presence of the editing cursor rune █
	if !strings.Contains(editView, "█") {
		t.Errorf("Edit mode view missing text input cursor █")
	}
}
