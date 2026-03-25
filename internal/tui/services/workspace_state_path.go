package services

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// BuildWorkspaceStatePath 返回当前工作区的会话状态文件路径。
func BuildWorkspaceStatePath(baseDir, workspaceRoot string) string {
	baseDir = strings.TrimSpace(baseDir)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if baseDir == "" || workspaceRoot == "" {
		return ""
	}

	hash := sha1.Sum([]byte(strings.ToLower(workspaceRoot)))
	return filepath.Join(baseDir, hex.EncodeToString(hash[:]), "session_state.json")
}
