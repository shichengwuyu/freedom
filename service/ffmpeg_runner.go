package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// === novel-workflow v2: composition-layer: ffmpeg runner ===
//
// RunFfmpegWithProgress 启动 ffmpeg 子进程，按 `-progress pipe:1` 解析进度行，
// 把当前步骤（1-5）+ lastMessage 写到 progressCallback 回调里。
//
// 5 步骤（composition-layer spec）：
//   1. 归一化所有镜头视频（libx264 + aac + yuv420p）
//   2. 按分镜顺序拼接（concat demuxer）
//   3. 混音（配音 + BGM；amix filter）
//   4. 烧字幕（ass filter；libass）
//   5. 输出最终 mp4

// ProgressEvent ffmpeg 进度事件。
type ProgressEvent struct {
	Step        int    // 1-5
	TotalSteps  int    // 固定 5
	LastMessage string // 例 "归一化镜头 3 / 8"
	OutTimeMs   int64  // ffmpeg -progress 解析的 out_time_ms（仅在第 4 步烧字幕时有意义）
	Done        bool   // ffmpeg 进程退出
	StderrTail  string // 失败时的 stderr 最后 N 行
}

// CheckFfmpegAvailable 检查 ffmpeg 是否可执行；返回 stdout 第一行（ffmpeg 版本信息）。
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

// RunFfmpegWithProgress 启动 ffmpeg + 解析 -progress pipe:1 输出。
//
// 参数：
//   - ctx: context（cancel 时 kill 子进程）
//   - binaryPath: ffmpeg 可执行路径
//   - args: ffmpeg 命令行参数（不含 binaryPath，不含 -progress pipe:1，自动加）
//   - step: 当前步骤（1-5）
//   - totalSteps: 总步骤（固定 5）
//   - lastMessage: 当前步骤的用户可见消息
//   - onProgress: 进度回调（每条 progress 行 + done + stderr 触发一次）
//
// 返回：
//   - exitCode: ffmpeg 退出码（0=成功）
//   - stderrTail: stderr 最后 20 行（用于失败诊断）
func RunFfmpegWithProgress(
	ctx context.Context,
	binaryPath string,
	args []string,
	step int,
	totalSteps int,
	lastMessage string,
	onProgress func(ProgressEvent),
) (exitCode int, stderrTail string, err error) {
	if binaryPath == "" {
		binaryPath = "ffmpeg"
	}
	// 自动加 -progress pipe:1 -nostats -hide_banner -loglevel error
	fullArgs := append([]string{
		"-hide_banner",
		"-loglevel", "error",
		"-progress", "pipe:1",
		"-nostats",
	}, args...)

	cmd := exec.CommandContext(ctx, binaryPath, fullArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return -1, "", err
	}
	if err := cmd.Start(); err != nil {
		return -1, "", err
	}

	// 启动期就发一个 step 事件（让 UI 立即反映）
	if onProgress != nil {
		onProgress(ProgressEvent{Step: step, TotalSteps: totalSteps, LastMessage: lastMessage})
	}

	// 启动 goroutine 读 stderr
	stderrBuf := []string{}
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		rdr := bufio.NewReader(stderr)
		for {
			line, e := rdr.ReadString('\n')
			if line != "" {
				stderrBuf = append(stderrBuf, strings.TrimRight(line, "\n"))
				// 仅保留最后 20 行
				if len(stderrBuf) > 20 {
					stderrBuf = stderrBuf[len(stderrBuf)-20:]
				}
			}
			if e != nil {
				if e != io.EOF {
					_ = e
				}
				return
			}
		}
	}()

	// 读 stdout（progress 行）
	var lastOutTimeMs int64
	go func() {
		rdr := bufio.NewReader(stdout)
		for {
			line, e := rdr.ReadString('\n')
			if line == "" {
				if e != nil {
					return
				}
				continue
			}
			line = strings.TrimRight(line, "\n")
			if line == "" {
				continue
			}
			// 解析 key=value 形式
			if strings.HasPrefix(line, "out_time_ms=") {
				val := strings.TrimPrefix(line, "out_time_ms=")
				var ms int64
				fmt.Sscanf(val, "%d", &ms)
				lastOutTimeMs = ms
			} else if line == "progress=continue" || line == "progress=end" {
				if onProgress != nil {
					onProgress(ProgressEvent{
						Step: step, TotalSteps: totalSteps,
						LastMessage: fmt.Sprintf("%s (已用 %d ms)", lastMessage, lastOutTimeMs),
						OutTimeMs:   lastOutTimeMs,
					})
				}
			}
			if e != nil {
				return
			}
		}
	}()

	// 等子进程退出
	waitErr := cmd.Wait()
	<-stderrDone

	tail := strings.Join(stderrBuf, "\n")
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return exitErr.ExitCode(), tail, nil
		}
		return -1, tail, waitErr
	}
	return 0, tail, nil
}
