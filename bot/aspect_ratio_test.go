package bot

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"tg-bawer/gemini"
)

func TestResolveAspectRatio_DefaultWhenNoImage(t *testing.T) {
	got := resolveAspectRatio("", nil)
	if got != defaultAspectRatio {
		t.Fatalf("expected %s, got %s", defaultAspectRatio, got)
	}
}

func TestResolveAspectRatio_UseRequested(t *testing.T) {
	got := resolveAspectRatio("16:9", nil)
	if got != "16:9" {
		t.Fatalf("expected requested ratio 16:9, got %s", got)
	}
}

func TestResolveAspectRatio_DetectNearestFromImage(t *testing.T) {
	imageBytes := mustMakePNG(t, 1000, 600) // 約 1.6667，最接近 16:9
	got := resolveAspectRatio("", []gemini.DownloadedImage{
		{Data: imageBytes, MimeType: "image/png"},
	})
	if got != "16:9" {
		t.Fatalf("expected 16:9, got %s", got)
	}
}

func mustMakePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	buffer := &bytes.Buffer{}
	if err := png.Encode(buffer, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("encode png failed: %v", err)
	}
	return buffer.Bytes()
}
