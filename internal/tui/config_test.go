package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/stretchr/testify/assert"
)

func TestConfigModel_Navigation(t *testing.T) {
	m := newConfigModel()

	m.Update(keyMsg('j'))
	if m.cursor != 1 {
		t.Errorf("Expected cursor at 1 after 'j', got %d", m.cursor)
	}

	m.Update(specialKeyMsg(tea.KeySpace))
	if m.items[1].Value != "false" {
		t.Errorf("Expected toggled boolean value 'false', got %s", m.items[1].Value)
	}

	m.cursor = 3
	m.Update(specialKeyMsg(tea.KeyEnter))
	if m.mode != configEditing {
		t.Errorf("Expected mode configEditing, got %v", m.mode)
	}

	m.Update(keyMsg('0'))
	if m.editBuffer != "150" {
		t.Errorf("Expected editBuffer '150', got %q", m.editBuffer)
	}
}

func TestConfigModelUsesResolvedValuesAndSharedValidation(t *testing.T) {
	values := localconfig.Defaults()
	values.OutputFormat = "json"
	values.MaxComplexity = 27
	values.Coverage = true
	values.HistoryEnabled = false
	m := newConfigModelWithValues(values)

	assert.Equal(t, "json", m.ConfigValue(localconfig.KeyOutputFormat))
	assert.Equal(t, "27", m.ConfigValue(localconfig.KeyMaxComplexity))
	assert.Equal(t, "true", m.ConfigValue(localconfig.KeyCoverage))
	assert.Equal(t, "false", m.ConfigValue(localconfig.KeyHistoryEnabled))

	m.cursor = 3
	m.mode = configEditing
	m.editBuffer = "0"
	m.Update(specialKeyMsg(tea.KeyEnter))
	assert.Equal(t, configEditing, m.mode)
	assert.Contains(t, m.validationError, "must be between 1")
	assert.Equal(t, "27", m.ConfigValue(localconfig.KeyMaxComplexity))
}

func TestConfigModel_View(t *testing.T) {
	m := newConfigModel()
	m.width, m.height = 100, 40

	view := m.render()

	expectedStrings := []string{
		"Settings",      // Title
		"General",       // Category divider
		"Output Format", // Item key
		"Quality Gate",  // Another category
	}

	for _, s := range expectedStrings {
		if !strings.Contains(view, s) {
			t.Errorf("Standard view missing expected string %q", s)
		}
	}

	m.cursor = 3
	m.Update(specialKeyMsg(tea.KeyEnter))

	editView := m.render()
	if !strings.Contains(editView, "█") {
		t.Errorf("Edit mode view missing text input cursor █")
	}
}
