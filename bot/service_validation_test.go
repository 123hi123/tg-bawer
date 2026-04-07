package bot

import (
	"testing"

	"tg-bawer/gemini"
)

func TestValidateServiceDefinitionRejectsChineseAPIKey(t *testing.T) {
	err := validateServiceDefinition(gemini.ServiceTypeStandard, "Joe", "小男娘", "", "", "", "")
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateServiceDefinitionAcceptsStandardAPIKey(t *testing.T) {
	err := validateServiceDefinition(gemini.ServiceTypeStandard, "Joe", "AIzaSyDemo_Key-123", "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateServiceDefinitionRejectsInvalidURL(t *testing.T) {
	err := validateServiceDefinition(gemini.ServiceTypeCustom, "proxy", "sk-demo", "abc", "", "", "")
	if err == nil {
		t.Fatalf("expected url validation error")
	}
}
