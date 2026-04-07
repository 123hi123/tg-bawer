package bot

import (
	"strings"
	"testing"

	"tg-bawer/gemini"
	"tg-bawer/grok"
)

func TestBuildTaskSummary_GoogleOnly(t *testing.T) {
	services := []resolvedGoogleService{
		{Config: gemini.ServiceConfig{Type: gemini.ServiceTypeStandard, Name: "svc1", Model: "gemini-3-pro-image-preview"}},
	}
	summary := buildTaskSummary(services, nil, nil, false)
	if !strings.Contains(summary, "共 1 個服務") {
		t.Fatalf("expected 1 service count, got: %s", summary)
	}
	if !strings.Contains(summary, "Google") {
		t.Fatalf("expected Google in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "gemini-3-pro-image-preview") {
		t.Fatalf("expected model name, got: %s", summary)
	}
}

func TestBuildTaskSummary_GoogleWithExtra(t *testing.T) {
	services := []resolvedGoogleService{
		{Config: gemini.ServiceConfig{Type: gemini.ServiceTypeStandard, Name: "svc1", Model: "gemini-3-pro-image-preview"}},
	}
	extras := []string{"gemini-3.1-flash-image-preview"}
	summary := buildTaskSummary(services, extras, nil, false)
	if !strings.Contains(summary, "共 2 個服務") {
		t.Fatalf("expected 2 service count, got: %s", summary)
	}
	if !strings.Contains(summary, "gemini-3.1-flash-image-preview") {
		t.Fatalf("expected extra model name, got: %s", summary)
	}
}

func TestBuildTaskSummary_GoogleDefaultModel(t *testing.T) {
	services := []resolvedGoogleService{
		{Config: gemini.ServiceConfig{Type: gemini.ServiceTypeStandard, Name: "svc1"}},
	}
	summary := buildTaskSummary(services, nil, nil, false)
	if !strings.Contains(summary, gemini.DefaultImageModel) {
		t.Fatalf("expected default model name %s, got: %s", gemini.DefaultImageModel, summary)
	}
}

func TestBuildTaskSummary_GrokOnly(t *testing.T) {
	gc := grok.NewClient("key", "", "", "", "")
	summary := buildTaskSummary(nil, nil, gc, false)
	if !strings.Contains(summary, "共 2 個服務") {
		t.Fatalf("expected 2 services (image+video), got: %s", summary)
	}
	if !strings.Contains(summary, grok.DefaultImgModel) {
		t.Fatalf("expected grok img model, got: %s", summary)
	}
	if !strings.Contains(summary, grok.DefaultVideoModel) {
		t.Fatalf("expected grok video model, got: %s", summary)
	}
}

func TestBuildTaskSummary_GrokWithImages(t *testing.T) {
	gc := grok.NewClient("key", "", "", "", "")
	summary := buildTaskSummary(nil, nil, gc, true)
	if !strings.Contains(summary, grok.DefaultEditModel) {
		t.Fatalf("expected grok edit model when hasImages=true, got: %s", summary)
	}
}

func TestBuildTaskSummary_AllTasks(t *testing.T) {
	services := []resolvedGoogleService{
		{Config: gemini.ServiceConfig{Type: gemini.ServiceTypeStandard, Name: "svc1", Model: "gemini-3-pro-image-preview"}},
	}
	extras := []string{"gemini-3.1-flash-image-preview"}
	gc := grok.NewClient("key", "", "", "", "")
	summary := buildTaskSummary(services, extras, gc, false)
	if !strings.Contains(summary, "共 4 個服務") {
		t.Fatalf("expected 4 services, got: %s", summary)
	}
	if !strings.Contains(summary, "🌐") {
		t.Fatal("expected Google icon")
	}
	if !strings.Contains(summary, "⚡") {
		t.Fatal("expected Flash icon")
	}
	if !strings.Contains(summary, "🎭") {
		t.Fatal("expected Grok image icon")
	}
	if !strings.Contains(summary, "🎬") {
		t.Fatal("expected Grok video icon")
	}
}

func TestBuildTaskSummary_Empty(t *testing.T) {
	summary := buildTaskSummary(nil, nil, nil, false)
	if summary != "" {
		t.Fatalf("expected empty summary, got: %s", summary)
	}
}

func TestBuildCaptionWithPrompt_Normal(t *testing.T) {
	caption := buildCaptionWithPrompt("🌐 Google 圖片", "畫一隻貓")
	if !strings.Contains(caption, "🌐 Google 圖片") {
		t.Fatalf("expected label in caption, got: %s", caption)
	}
	if !strings.Contains(caption, "<blockquote expandable>") {
		t.Fatalf("expected expandable blockquote, got: %s", caption)
	}
	if !strings.Contains(caption, "畫一隻貓") {
		t.Fatalf("expected prompt in caption, got: %s", caption)
	}
	if !strings.Contains(caption, "</blockquote>") {
		t.Fatalf("expected closing blockquote, got: %s", caption)
	}
}

func TestBuildCaptionWithPrompt_EmptyPrompt(t *testing.T) {
	caption := buildCaptionWithPrompt("🌐 Google 圖片", "")
	if caption != "🌐 Google 圖片" {
		t.Fatalf("expected label only when prompt is empty, got: %s", caption)
	}
}

func TestBuildCaptionWithPrompt_HTMLEscape(t *testing.T) {
	caption := buildCaptionWithPrompt("label", "<script>alert('xss')</script>")
	if strings.Contains(caption, "<script>") {
		t.Fatalf("expected HTML-escaped prompt, got: %s", caption)
	}
	if !strings.Contains(caption, "&lt;script&gt;") {
		t.Fatalf("expected escaped angle brackets, got: %s", caption)
	}
}

func TestBuildCaptionWithPrompt_LongPrompt(t *testing.T) {
	longPrompt := strings.Repeat("a", 1000)
	caption := buildCaptionWithPrompt("label", longPrompt)
	// The prompt should be truncated to 900 runes + "..."
	if !strings.Contains(caption, "...") {
		t.Fatalf("expected truncation indicator, got: %s", caption)
	}
	// Verify the prompt content is truncated (900 'a's not 1000)
	if strings.Contains(caption, strings.Repeat("a", 901)) {
		t.Fatalf("expected truncation at 900 runes")
	}
}

func TestBuildCaptionWithPrompt_MultiByteTruncation(t *testing.T) {
	// Use CJK characters (3 bytes each in UTF-8) near the boundary
	longPrompt := strings.Repeat("你", 950)
	caption := buildCaptionWithPrompt("label", longPrompt)
	if !strings.Contains(caption, "...") {
		t.Fatalf("expected truncation indicator, got: %s", caption)
	}
	// Verify no corrupted multi-byte characters by checking valid UTF-8
	for _, r := range caption {
		if r == '\uFFFD' {
			t.Fatalf("found replacement character, indicating corrupted UTF-8")
		}
	}
}

func TestIsLongPrompt(t *testing.T) {
	short := strings.Repeat("a", 899)
	if isLongPrompt(short) {
		t.Fatalf("expected short prompt (899 runes) to not be long")
	}

	exact := strings.Repeat("a", 900)
	if !isLongPrompt(exact) {
		t.Fatalf("expected prompt at threshold (900 runes) to be long")
	}

	long := strings.Repeat("你", 900)
	if !isLongPrompt(long) {
		t.Fatalf("expected CJK prompt at threshold to be long")
	}
}

func TestBuildCaptionForResult_Short(t *testing.T) {
	caption := buildCaptionForResult("🌐 Google 圖片 | 文生圖", "畫一隻貓")
	if !strings.Contains(caption, "🌐 Google 圖片 | 文生圖") {
		t.Fatalf("expected label in caption, got: %s", caption)
	}
	if !strings.Contains(caption, "<blockquote expandable>") {
		t.Fatalf("expected expandable blockquote for short prompt, got: %s", caption)
	}
	if !strings.Contains(caption, "畫一隻貓") {
		t.Fatalf("expected prompt in caption, got: %s", caption)
	}
}

func TestBuildCaptionForResult_Long(t *testing.T) {
	longPrompt := strings.Repeat("a", 1000)
	caption := buildCaptionForResult("🌐 Google 圖片 | 文生圖", longPrompt)
	if caption != "🌐 Google 圖片 | 文生圖" {
		t.Fatalf("expected label only for long prompt, got: %s", caption)
	}
}

func TestGenerationTypeLabel(t *testing.T) {
	tests := []struct {
		hasImages bool
		mediaType string
		expected  string
	}{
		{false, "image", " | 文生圖"},
		{true, "image", " | 圖生圖"},
		{false, "video", " | 文生影片"},
		{true, "video", " | 圖生影片"},
	}
	for _, tt := range tests {
		got := generationTypeLabel(tt.hasImages, tt.mediaType)
		if got != tt.expected {
			t.Errorf("generationTypeLabel(%v, %q) = %q, want %q", tt.hasImages, tt.mediaType, got, tt.expected)
		}
	}
}
