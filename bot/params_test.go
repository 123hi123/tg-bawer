package bot

import "testing"

func TestParseTextParams_WithSingleImageFlag(t *testing.T) {
	params := parseTextParams("翻譯這張圖 @16:9 @4K @s")

	if params.Prompt != "翻譯這張圖" {
		t.Fatalf("unexpected prompt: %q", params.Prompt)
	}
	if params.AspectRatio != "16:9" {
		t.Fatalf("unexpected ratio: %q", params.AspectRatio)
	}
	if params.Quality != "4K" {
		t.Fatalf("unexpected quality: %q", params.Quality)
	}
	if !params.SingleImageFromGroup {
		t.Fatalf("expected SingleImageFromGroup=true")
	}
}

func TestBuildRetryQualities_NoDowngrade(t *testing.T) {
	qualities := buildRetryQualities("4K")
	if len(qualities) != 6 {
		t.Fatalf("expected 6 retry qualities, got %d", len(qualities))
	}

	for i, quality := range qualities {
		if quality != "4K" {
			t.Fatalf("retry #%d unexpected quality: %s", i, quality)
		}
	}
}
