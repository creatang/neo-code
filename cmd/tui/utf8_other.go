//go:build !windows

package main
// setUTF8Mode 在非 Windows 系统上是空操作。
// Linux 和 macOS 默认使用 UTF-8 编码，无需额外设置。
func setUTF8Mode() {}
