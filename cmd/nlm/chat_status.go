package main

import (
	"os"
	"strings"
	"time"
)

const chatStaleAfter = 90 * time.Second

func chatPartialPath(sessionPath string) string {
	return strings.TrimSuffix(sessionPath, ".json") + ".partial.jsonl"
}

func resolveChatStatus(status string, at time.Time, pid int, path string, now time.Time) string {
	switch status {
	case chatStatusRunning:
		if info, err := os.Stat(chatPartialPath(path)); err == nil && now.Sub(info.ModTime()) < chatStaleAfter {
			return "generating"
		}
		if chatWriterAlive(pid) {
			return "generating"
		}
		return "interrupted"
	case chatStatusIncomplete:
		return "truncated"
	case chatStatusError:
		return "failed"
	default:
		return ""
	}
}
