package connector

import (
	"sort"
	"strings"

	"maunium.net/go/mautrix/event"
)

type mediaAttachment struct {
	Index         int
	URL           string
	MimeType      string
	EncryptedFile *event.EncryptedFileInfo
	FileName      string
}

func selectMediaAttachments(attachments []mediaAttachment, policy *MediaUnderstandingAttachmentsConfig) []mediaAttachment {
	if len(attachments) == 0 {
		return nil
	}

	mode := ""
	prefer := ""
	max := 1
	if policy != nil {
		mode = strings.TrimSpace(strings.ToLower(policy.Mode))
		prefer = strings.TrimSpace(strings.ToLower(policy.Prefer))
		if policy.MaxAttachments > 0 {
			max = policy.MaxAttachments
		}
	}
	if mode == "" {
		mode = "first"
	}

	ordered := make([]mediaAttachment, 0, len(attachments))
	ordered = append(ordered, attachments...)

	switch prefer {
	case "last":
		for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
			ordered[i], ordered[j] = ordered[j], ordered[i]
		}
	case "path":
		sort.SliceStable(ordered, func(i, j int) bool {
			left := strings.ToLower(strings.TrimSpace(ordered[i].FileName))
			right := strings.ToLower(strings.TrimSpace(ordered[j].FileName))
			if left == "" && right == "" {
				return ordered[i].Index < ordered[j].Index
			}
			if left == "" {
				return false
			}
			if right == "" {
				return true
			}
			if left == right {
				return ordered[i].Index < ordered[j].Index
			}
			return left < right
		})
	case "url":
		sort.SliceStable(ordered, func(i, j int) bool {
			left := strings.ToLower(strings.TrimSpace(ordered[i].URL))
			right := strings.ToLower(strings.TrimSpace(ordered[j].URL))
			if left == right {
				return ordered[i].Index < ordered[j].Index
			}
			return left < right
		})
	}

	if mode == "all" {
		if max > 0 && len(ordered) > max {
			return ordered[:max]
		}
		return ordered
	}

	if len(ordered) == 0 {
		return nil
	}
	return ordered[:1]
}
