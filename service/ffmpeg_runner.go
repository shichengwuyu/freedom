package service

import (
	"fmt"
	"os/exec"
	"strings"
)

// CheckFfmpegAvailable 检查 ffmpeg 是否可执行；返回 stdout 第一行（ffmpeg 版本信息）。
//
// novel-workflow v2 引入。main.go 启动时调一次，缺失只 warn，不阻塞。
// 真正的合成调用（service/novel_composition.go）会再次 exec 并捕获详细错误。
func CheckFfmpegAvailable(binaryPath string) (string, error) {
	if binaryPath == "" {
		binaryPath = "ffmpeg"
	}
	out, err := exec.Command(binaryPath, "-version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("exec %s -version failed: %w", binaryPath, err)
	}
	firstLine := strings.SplitN(string(out), "\n", 2)[0]
	return firstLine, nil
}
