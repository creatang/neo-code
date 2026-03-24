//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

const utf8CodePage = 65001

func setUTF8Mode() {
	if err := windows.SetConsoleOutputCP(utf8CodePage); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 设置控制台输出编码失败: %v\n", err)
	}
	if err := windows.SetConsoleCP(utf8CodePage); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 设置控制台输入编码失败: %v\n", err)
	}
}
