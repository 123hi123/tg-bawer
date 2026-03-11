package bot

import (
	"strings"
	"testing"

	"tg-bawer/gemini"
	"tg-bawer/grok"
)

func TestBuildTaskSummary_GoogleOnly(t *testing.T) {
	services := []gemini.ServiceConfig{
		{Type: gemini.ServiceTypeStandard, Name: "svc1", Model: "gemini-3-pro-image-preview"},
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
	services := []gemini.ServiceConfig{
		{Type: gemini.ServiceTypeStandard, Name: "svc1", Model: "gemini-3-pro-image-preview"},
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
	services := []gemini.ServiceConfig{
		{Type: gemini.ServiceTypeStandard, Name: "svc1"},
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
	services := []gemini.ServiceConfig{
		{Type: gemini.ServiceTypeStandard, Name: "svc1", Model: "gemini-3-pro-image-preview"},
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
	if !strings.Contains(summary, "🤖") {
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
	// The prompt should be truncated to 900 chars + "..."
	if len(caption) > 1024 {
		t.Fatalf("caption exceeds 1024 chars: len=%d", len(caption))
	}
	if !strings.Contains(caption, "...") {
		t.Fatalf("expected truncation indicator, got: %s", caption)
	}
}
