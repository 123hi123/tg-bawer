package bot

import (
	"strings"

	"tg-bawer/gemini"
)

const defaultAspectRatio = "1:1"

func resolveAspectRatio(requested string, downloadedImages []gemini.DownloadedImage) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested
	}

	if len(downloadedImages) == 0 {
		return defaultAspectRatio
	}

	imageInfo, err := gemini.GetImageInfo(downloadedImages[0].Data)
	if err != nil || imageInfo == nil || imageInfo.AspectRatio == "" {
		return defaultAspectRatio
	}

	return imageInfo.AspectRatio
}

func ratioDisplayText(requested, resolved string, imageCount int) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return resolved
	}
	if imageCount > 0 {
		return resolved + " (自動偵測)"
	}
	return resolved + " (預設)"
}
