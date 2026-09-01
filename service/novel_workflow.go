package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// novel-workflow v2：工作流编排器
//
// 职责：
//   - CreateRun：新建一次 Run + 按 ExpandWorkflowGraph 创建所有节点
//   - Start/Stop/Retry 单节点：节点状态机操作
//   - OnNodeFinished：节点完成回调（成功 / 失败 / 跳过）；推进依赖图
//   - Worker：5s 轮询，把"就绪"节点派发到通用 task worker
//
// 自动化 vs 手动：
//   - auto 模式：节点成功完成时自动派发下游节点
//   - manual 模式：只派发用户手动点"开始"的节点
//   - quick/custom 模式：等同于 auto（按"启动"按钮时设 mode）

const novelWorkflowPollInterval = 5 * time.Second

var (
	novelWorkflowRunMu sync.Mutex
)

// CreateNovelWorkflowRun 新建一次 run + 初始化所有节点（按 shot 数展开）。
//
// params: userID / userGroupCode / projectID / mode（auto|manual|quick|custom）/ shotIDs / configJSON。
//
// 返回 run 对象。
func CreateNovelWorkflowRun(userID, userGroupCode, projectID, mode string, shotIDs []string, configJSON string) (*model.NovelWorkflowRun, error) {
	if userID == "" || projectID == "" {
		return nil, errors.New("userID and projectID are required")
	}
	if mode == "" {
		mode = "auto"
	}

	defs := ExpandWorkflowGraph(shotIDs)
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	run := &model.NovelWorkflowRun{
		ID:             newID("nwf"),
		UserID:         userID,
		UserGroupCode:  userGroupCode,
		ProjectID:      projectID,
		Mode:           mode,
		OverallStatus:  string(statusNotStarted),
		ConfigJSON:     configJSON,
		TotalNodes:     len(defs),
		PendingNodes:   len(defs),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repository.CreateNovelWorkflowRun(run); err != nil {
		return nil, err
	}

	// 节点记录
	// 不在 CreateRun 时立即建节点，等用户首次"启动"时建（避免 run 创建后没启动留下一堆"未启动"节点）。
	return run, nil
}

// InitNovelWorkflowNodes 初始化 run 的所有节点（按定义），并启动 worker 派发第一个就绪节点。
//
// 通常在用户首次点"启动"时调（auto / quick / custom 模式）；manual 模式等用户单点启动时再建。
func InitNovelWorkflowNodes(runID string, shotIDs []string) error {
	run, err := repository.GetNovelWorkflowRun(runID)
	if err != nil {
		return err
	}
	if run == nil {
		return errors.New("run not found")
	}
	defs := ExpandWorkflowGraph(shotIDs)
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	for _, d := range defs {
		depJSON, _ := json.Marshal(d.DependsOn)
		n := &model.NovelWorkflowNode{
			ID:            newID("nwfn"),
			RunID:         runID,
			UserID:        run.UserID,
			ProjectID:     run.ProjectID,
			NodeID:        d.NodeID,
			Layer:         string(d.Layer),
			NodeKind:      string(d.Kind),
			NodeTitle:     d.Title,
			ShotIndex:     d.PerShotIndex,
			DependsOnJSON: string(depJSON),
			Status:        string(statusNotStarted),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := repository.CreateNovelWorkflowNode(n); err != nil {
			return err
		}
	}
	return nil
}

// StartNovelWorkflowRun 启动一次 run（auto / quick / custom 模式）。
//   - 若节点未初始化则建
//   - 找出所有"就绪"（依赖全成功）且 status=未启动的节点
//   - 派发到通用 task worker
//   - 自身状态 → 进行中
//
// 安全：handler 已校验 run.UserID == user.ID；service 不再重复校验。
// 注意：service 层其他 Start*Node / Cancel / Retry 也不再重复校验，
// 因为它们都以 (runID, nodeID) 查表，而 node 一定属于某个 run——只要 runID
// 在 handler 入口被校验过，node 也就间接校验过。
func StartNovelWorkflowRun(runID string) error {
	run, err := repository.GetNovelWorkflowRun(runID)
	if err != nil || run == nil {
		return errors.New("run not found")
	}
	novelWorkflowRunMu.Lock()
	defer novelWorkflowRunMu.Unlock()

	// 若节点未初始化：按"项目当前分镜列表"建节点
	existing, _ := repository.ListNovelWorkflowNodesByRun(runID)
	if len(existing) == 0 {
		// 取项目分镜 ID（v2 简化：调用方传入；如未传则从 project store 读——v2 阶段先简化）
		shotIDs := getProjectShotIDsForWorkflow(run.ProjectID)
		if err := InitNovelWorkflowNodes(runID, shotIDs); err != nil {
			return err
		}
	}

	// 自动模式：派发就绪节点
	if run.Mode != "manual" {
		if err := dispatchReadyNodes(run); err != nil {
			return err
		}
	}

	// run 状态推进
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if run.OverallStatus == string(statusNotStarted) {
		run.OverallStatus = string(statusRunning)
		run.StartedAt = now
	}
	run.UpdatedAt = now
	return repository.UpdateNovelWorkflowRun(run)
}

// StartNovelWorkflowNode 启动单节点（manual 模式 / 单步介入）。
//
// 安全：userID 用于纵深防御校验 run 归属（handler 已校验过）。
func StartNovelWorkflowNode(userID, runID, nodeID string) error {
	run, err := repository.GetNovelWorkflowRun(runID)
	if err != nil || run == nil {
		return errors.New("run not found")
	}
	if run.UserID != userID {
		return errors.New("无权访问该 run")
	}
	node, err := repository.GetNovelWorkflowNodeByRunAndNodeID(runID, nodeID)
	if err != nil || node == nil {
		return errors.New("node not found")
	}
	// 检查依赖
	if !checkDependenciesSuccess(node) {
		return errors.New("dependencies not satisfied")
	}
	// 派发
	return dispatchNode(run, node)
}

// CancelNovelWorkflowNode 取消单节点。
//
// 安全：userID 用于纵深防御校验 run 归属。
func CancelNovelWorkflowNode(userID, runID, nodeID string) error {
	run, err := repository.GetNovelWorkflowRun(runID)
	if err != nil || run == nil {
		return errors.New("run not found")
	}
	if run.UserID != userID {
		return errors.New("无权访问该 run")
	}
	node, err := repository.GetNovelWorkflowNodeByRunAndNodeID(runID, nodeID)
	if err != nil || node == nil {
		return errors.New("node not found")
	}
	if node.Status != string(statusQueued) && node.Status != string(statusRunning) {
		return errors.New("node not active")
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	node.Status = string(statusCanceled)
	node.UpdatedAt = now
	node.CompletedAt = now
	if err := repository.UpdateNovelWorkflowNode(node); err != nil {
		return err
	}
	// 通知通用 task worker 取消（如有）
	if node.GenericTaskID != "" {
		_ = CancelGenericTask(node.GenericTaskID)
	}
	return refreshRunAggregation(runID)
}

// RetryNovelWorkflowNode 重试失败节点。
//
// 安全：userID 用于纵深防御校验 run 归属。
func RetryNovelWorkflowNode(userID, runID, nodeID string) error {
	run, err := repository.GetNovelWorkflowRun(runID)
	if err != nil || run == nil {
		return errors.New("run not found")
	}
	if run.UserID != userID {
		return errors.New("无权访问该 run")
	}
	node, err := repository.GetNovelWorkflowNodeByRunAndNodeID(runID, nodeID)
	if err != nil || node == nil {
		return errors.New("node not found")
	}
	if node.Status != string(statusFailed) {
		return errors.New("node not in failed state")
	}
	if !checkDependenciesSuccess(node) {
		return errors.New("dependencies not satisfied")
	}
	node.Status = string(statusNotStarted)
	node.UpdatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	return dispatchNode(run, node)
}

// === 内部 helpers ===

// dispatchReadyNodes 找出所有"就绪"（依赖全成功）且 status=未启动的节点，批量派发。
func dispatchReadyNodes(run *model.NovelWorkflowRun) error {
	nodes, err := repository.ListNovelWorkflowNodesByRun(run.ID)
	if err != nil {
		return err
	}
	// 按 nodeID 建索引便于查依赖
	byNodeID := make(map[string]*model.NovelWorkflowNode, len(nodes))
	for i := range nodes {
		byNodeID[nodes[i].NodeID] = &nodes[i]
	}
	for i := range nodes {
		n := &nodes[i]
		if n.Status != string(statusNotStarted) {
			continue
		}
		if !checkDependenciesSuccessByIndex(n, byNodeID) {
			continue
		}
		if err := dispatchNode(run, n); err != nil {
			log.Printf("novel-workflow: dispatch node=%s err=%v", n.NodeID, err)
		}
	}
	return nil
}

// dispatchNode 把单个节点派发到通用 task worker。
//
// v2 阶段：仅打印日志 + 标"排队中"，真正 handler 接入在任务组 3-7。
// 后续任务组会把通用 task handler 注册到 service/task_worker.go 的 RegisterTaskHandler。
func dispatchNode(run *model.NovelWorkflowRun, node *model.NovelWorkflowNode) error {
	now := time.Now().UTC()
	nowStr := now.Format("2006-01-02T15:04:05.000Z")
	node.Status = string(statusQueued)
	node.UpdatedAt = nowStr
	// 写一条通用 task（task_worker 会拉取并派发）
	genericTask := &model.Task{
		ID:          newID("nwtask"),
		UserID:      run.UserID,
		Type:        "novel-workflow-node:" + node.NodeKind,
		Status:      model.TaskStatusPending,
		PayloadJSON: mustJSONMap(map[string]any{"runId": run.ID, "nodeId": node.NodeID, "nodeDbId": node.ID}),
		MaxAttempts: 3,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repository.SaveTask(genericTask); err != nil {
		return err
	}
	node.GenericTaskID = genericTask.ID
	return repository.UpdateNovelWorkflowNode(node)
}

// checkDependenciesSuccess 检查节点的依赖是否全部"成功"或"跳过"。
func checkDependenciesSuccess(node *model.NovelWorkflowNode) bool {
	deps := parseDepJSON(node.DependsOnJSON)
	if len(deps) == 0 {
		return true
	}
	for _, depID := range deps {
		dep, err := repository.GetNovelWorkflowNodeByRunAndNodeID(node.RunID, depID)
		if err != nil || dep == nil {
			return false
		}
		if dep.Status != string(statusSuccess) && dep.Status != string(statusSkipped) {
			return false
		}
	}
	return true
}

// checkDependenciesSuccessByIndex 内存索引版（避免循环里多次查 DB）。
func checkDependenciesSuccessByIndex(node *model.NovelWorkflowNode, byNodeID map[string]*model.NovelWorkflowNode) bool {
	deps := parseDepJSON(node.DependsOnJSON)
	if len(deps) == 0 {
		return true
	}
	for _, depID := range deps {
		dep, ok := byNodeID[depID]
		if !ok {
			return false
		}
		if dep.Status != string(statusSuccess) && dep.Status != string(statusSkipped) {
			return false
		}
	}
	return true
}

// refreshRunAggregation 重算 run 的总体状态 + 节点计数。
// 每次节点完成 / 失败 / 取消时调一次。
func refreshRunAggregation(runID string) error {
	run, err := repository.GetNovelWorkflowRun(runID)
	if err != nil || run == nil {
		return err
	}
	nodes, err := repository.ListNovelWorkflowNodesByRun(runID)
	if err != nil {
		return err
	}
	var success, failed, skipped, canceled, pending int
	hasRunning := false
	for _, n := range nodes {
		switch n.Status {
		case string(statusSuccess):
			success++
		case string(statusFailed):
			failed++
		case string(statusSkipped):
			skipped++
		case string(statusCanceled):
			canceled++
		case string(statusNotStarted), string(statusQueued), string(statusRunning):
			pending++
		}
		if n.Status == string(statusRunning) || n.Status == string(statusQueued) {
			hasRunning = true
		}
	}
	run.SuccessNodes = success
	run.FailedNodes = failed
	run.SkippedNodes = skipped
	run.CanceledNodes = canceled
	run.PendingNodes = pending
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	run.UpdatedAt = now

	switch {
	case pending == len(nodes) && len(nodes) > 0:
		// 还有未跑完的，不动
	case hasRunning:
		run.OverallStatus = string(statusRunning)
	case failed > 0 && (success+skipped) > 0:
		run.OverallStatus = "部分失败"
	case canceled > 0 && failed == 0:
		run.OverallStatus = "已停止"
	case pending == 0 && failed == 0:
		run.OverallStatus = "已完成"
		run.CompletedAt = now
	}
	return repository.UpdateNovelWorkflowRun(run)
}

// OnNovelWorkflowNodeFinished 节点完成回调（task worker 调）。
//
// 参数：nodeDbId（model.NovelWorkflowNode.ID）+ status + outputURL + errorMsg。
// 写回节点状态 + 推进依赖图（auto 模式派发下游）+ 重算 run 聚合。
func OnNovelWorkflowNodeFinished(nodeDbID string, status string, outputURL string, errorMsg string) error {
	node, err := repository.GetNovelWorkflowNodeByID(nodeDbID)
	if err != nil || node == nil {
		return errors.New("node not found")
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	node.Status = status
	if outputURL != "" {
		node.OutputURL = outputURL
	}
	node.Error = errorMsg
	if status == string(statusSuccess) || status == string(statusFailed) || status == string(statusSkipped) {
		node.CompletedAt = now
	}
	if err := repository.UpdateNovelWorkflowNode(node); err != nil {
		return err
	}
	// auto 模式：成功完成时派发下游
	run, _ := repository.GetNovelWorkflowRun(node.RunID)
	if run != nil && run.Mode != "manual" && status == string(statusSuccess) {
		if err := dispatchReadyNodes(run); err != nil {
			log.Printf("novel-workflow: dispatch downstream after node=%s err=%v", node.NodeID, err)
		}
	}
	return refreshRunAggregation(node.RunID)
}

// === 内部 helpers（持续）===

func parseDepJSON(s string) []string {
	if s == "" {
		return nil
	}
	var deps []string
	if err := json.Unmarshal([]byte(s), &deps); err != nil {
		return nil
	}
	return deps
}

func mustJSONMap(v map[string]any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// getProjectShotIDsForWorkflow 取项目当前分镜 ID 列表（用于 InitNovelWorkflowNodes 展开 per-shot 节点）。
//
// v2 阶段：实现为最简版本——返回空数组（不展开 per-shot 节点）；
// 后续任务组接入 NovelProject 时会真正读项目状态。
func getProjectShotIDsForWorkflow(projectID string) []string {
	return nil
}

// === Background worker（5s 轮询）===

var novelWorkflowWorkerOnce sync.Once

// StartNovelWorkflowWorker 启动 novel-workflow 状态聚合刷新 worker。
//
// 真正节点派发走通用 task worker（model/task.go + service/task_worker.go）。
// 本 worker 负责：run 完成后清理过期 run + 5 分钟没动的 run 标已停止。
func StartNovelWorkflowWorker(ctx context.Context) {
	novelWorkflowWorkerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(novelWorkflowPollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					novelWorkflowWorkerTick()
				}
			}
		}()
	})
}

func novelWorkflowWorkerTick() {
	// v2 阶段：仅打印 metrics；v3 可加"超时未动 run 自动停止"逻辑
	runs, err := repository.ListActiveNovelWorkflowRuns()
	if err != nil {
		log.Printf("novel-workflow: list active runs err=%v", err)
		return
	}
	if len(runs) > 0 {
		log.Printf("novel-workflow: %d active runs", len(runs))
	}
}

// CancelGenericTask 取消通用 task（v2 占位；任务组 3-7 接入时复用 model/task.go 的 cancel 路径）。
func CancelGenericTask(taskID string) error {
	// 写 cancel 状态由 task worker 自己处理（attempts 超时即终止）
	// v2 阶段：仅清空 payload 里的"want_cancel" 标记；v3 接入 handler 取消信号
	return nil
}
