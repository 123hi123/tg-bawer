package bot

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"tg-bawer/gemini"
	"tg-bawer/grok"
)

func containsCJK(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return true
		}
	}
	return false
}

func isASCIIWithoutWhitespace(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r > unicode.MaxASCII || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func validateURLField(fieldName, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%s 不能為空", fieldName)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s 格式錯誤，必須是 http:// 或 https:// 開頭的完整網址", fieldName)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s 格式錯誤，必須是 http:// 或 https:// 開頭的完整網址", fieldName)
	}
	return nil
}

func validateASCIIField(fieldName, value string) error {
	if !isASCIIWithoutWhitespace(value) || containsCJK(value) {
		return fmt.Errorf("%s 格式錯誤，不能包含中文或空白", fieldName)
	}
	return nil
}

func validateServiceDefinition(serviceType, name, apiKey, baseURL, projectID, location, model string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("服務名稱不能為空")
	}

	switch serviceType {
	case gemini.ServiceTypeStandard:
		return validateASCIIField("API Key", apiKey)
	case gemini.ServiceTypeCustom:
		if err := validateURLField("Base URL", baseURL); err != nil {
			return err
		}
		return validateASCIIField("API Key", apiKey)
	case gemini.ServiceTypeVertex:
		if err := validateASCIIField("API Key", apiKey); err != nil {
			return err
		}
		if strings.TrimSpace(projectID) != "" {
			if err := validateASCIIField("Project ID", projectID); err != nil {
				return err
			}
		}
		if strings.TrimSpace(location) != "" {
			if err := validateASCIIField("Location", location); err != nil {
				return err
			}
		}
		if strings.TrimSpace(model) != "" {
			if err := validateASCIIField("Model", model); err != nil {
				return err
			}
		}
		if strings.TrimSpace(baseURL) != "" {
			if err := validateURLField("Base URL", baseURL); err != nil {
				return err
			}
		}
		return nil
	case grok.ServiceTypeGrok:
		if err := validateURLField("Base URL", baseURL); err != nil {
			return err
		}
		return validateASCIIField("API Key", apiKey)
	default:
		return nil
	}
}

func isDuplicateServiceNameErr(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "unique constraint failed: user_services.user_id, user_services.name")
}
