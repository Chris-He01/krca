# Knsight Agent Hub 设计文档（CloudWeGo Eino ADK 版）

## 1. 概览
Knsight Agent Hub 采用 CloudWeGo Eino ADK 作为核心编排框架，固定顶层与中间编排层，底层 Agent 可注册与发现。系统支持：
- Eino ADK 的多 Agent 协作、transfer/interrupt 语义与事件模型。
- MCP 工具接入（SSE 协议）与 OpenAI API 兼容外部 Agent。
- 企业级 SOP 与工作流编排能力（可配置）。

本设计对齐 `rca_demo/mcp_demo` 的核心逻辑：Supervisor 负责计划与协调，Inspect 通过 MCP 采集指标，Vision 生成可视化描述，最终输出 RCA 报告。

## 2. 目标与约束
### 目标
1. 以 Eino ADK 作为统一编排内核，避免自造轮子。
2. 提供注册中心能力，支持 Agent 发现与保活。
3. 支持配置化接入外部 OpenAI 格式 Agent 与 Python 工具 Agent。
4. 支持企业级 SOP/Workflow 扩展与复用。

### 约束
- 顶层（Supervisor）与中间编排层固定。
- 底层 Agent 通过注册或配置接入。
- 事件模型统一为 Eino ADK `AgentEvent`。

## 3. 架构总览
```
┌──────────────────────────────────────────────────────────────┐
│                         Top Layer (固定)                     │
│  Supervisor Agent (Eino ADK ChatModelAgent)                   │
└─────────────────────────────┬────────────────────────────────┘
                              │ transfer / interrupt
┌─────────────────────────────v────────────────────────────────┐
│                       Middle Layer (固定)                    │
│  Eino ADK Runner + CheckPointStore + Event Stream            │
│  Tool Router (MCP / Approval / Custom Tools)                 │
└─────────────────────────────┬────────────────────────────────┘
                              │
┌─────────────────────────────v────────────────────────────────┐
│                      Bottom Layer (可插拔)                   │
│  InspectAgent / VisionAgent / ReportAgent / ExternalAgent     │
│  - Eino ADK SubAgent                                           │
│  - External OpenAI API Agent                                  │
│  - Python Tool Agent (HTTP/MCP)                               │
└─────────────────────────────┬────────────────────────────────┘
                              │
┌─────────────────────────────v────────────────────────────────┐
│                     Registry / Discovery                     │
│  AgentCard 注册 / 心跳 / 发现                                 │
└──────────────────────────────────────────────────────────────┘
```

## 4. 核心概念（与 Eino ADK 对齐）
### 4.1 Agent
- 使用 `adk.ChatModelAgent` 实现 LLM Agent。
- 通过 `supervisor.New` 形成 Supervisor/Worker 结构。

### 4.2 Transfer
- Eino ADK 内置 `transfer_to_agent` 工具。
- 由 Supervisor 决定任务转交给某个 SubAgent。

### 4.3 Interrupt / Resume
- 使用 Eino ADK 中断语义：
  - 工具调用触发 `tool.Interrupt(...)`。
  - Runner 捕获中断并写入 CheckPointStore。
  - Resume 时使用 `ResumeWithParams` 传入目标 interrupt id。

### 4.4 MCP 工具
- 使用 `eino-ext/components/tool/mcp` 适配 MCP SSE 工具。
- 通过 `mark3labs/mcp-go` 客户端初始化与拉取工具。

## 5. 与 `rca_demo/mcp_demo` 的对齐
| mcp_demo 角色 | Knsight Eino ADK 对应实现 |
| --- | --- |
| InsightAgent | Supervisor（ChatModelAgent） |
| InspectAgent | SubAgent + MCP 工具 |
| VisionAgent | SubAgent（视觉/图表描述） |
| MCP Client | `tool/mcp` + SSE client |
| EventType/AgentEvent | `adk.AgentEvent` |

## 6. 关键流程
### 6.1 启动
1. Registry 启动，等待 Agent 注册。
2. MCP Server 与 LLM Server 启动。
3. Hub 启动，加载配置并构建 Supervisor + SubAgents。
4. 从 Registry 拉取外部 Agent（OpenAI 兼容）。

### 6.2 运行
1. 用户请求 `/v1/chat`。
2. Runner 执行 Supervisor。
3. Supervisor 通过 `transfer_to_agent` 交给 Inspect/Vision 等子 Agent。
4. Inspect 调 MCP 工具拿指标。
5. Supervisor 汇总输出 RCA 报告。

### 6.3 Interrupt/Resume
1. Supervisor 调用 `request_approval` 工具触发中断。
2. Runner 写 CheckPoint。
3. 用户调用 `/v1/workflow/resume`，传入 `target_id` 和 `data`。
4. Runner 恢复执行并返回最终结果。

## 7. 配置设计
### 7.1 Hub 配置（`configs/hub.json`）
```json
{
  "listen_addr": ":8080",
  "registry_url": "http://localhost:8081",
  "llm": {
    "base_url": "http://localhost:8090/v1",
    "model": "mock",
    "api_key": "mock"
  },
  "mcp": {
    "enabled": true,
    "sse_url": "http://localhost:8091/sse",
    "tool_names": ["server_metrics"]
  },
  "supervisor": {
    "name": "InsightSupervisor",
    "description": "Top-level coordinator for RCA workflows.",
    "instruction": "Delegate diagnostics to InspectAgent and visualization to VisionAgent.",
    "use_mcp": false
  },
  "agents": [
    {
      "name": "InspectAgent",
      "description": "MCP-enabled inspector",
      "instruction": "Use MCP tools to fetch metrics and return diagnostics.",
      "use_mcp": true
    },
    {
      "name": "VisionAgent",
      "description": "Visualization agent",
      "instruction": "Summarize chart intent and trends.",
      "use_mcp": false
    }
  ],
  "external_agents": []
}
```

### 7.2 AgentCard（Registry）
```json
{
  "id": "external-analyst",
  "name": "ExternalAnalyst",
  "version": "0.1.0",
  "description": "external openai agent",
  "capabilities": ["analysis"],
  "endpoint": "http://localhost:8093/v1",
  "protocol": "openai",
  "model": "mock"
}
```

## 8. 对外接口
### 8.1 `/v1/chat`
- `POST`
- 请求：
```json
{"message":"检查服务器内存指标并生成报告","run_id":"","stream":false}
```
- 响应：
```json
{"run_id":"...","output":"...","events":[...],"interrupts":[...]}
```

### 8.2 `/v1/workflow/resume`
- `POST`
- 请求：
```json
{"run_id":"<run_id>","target_id":"<interrupt_id>","data":"approved"}
```

### 8.3 `/v1/workflow/state/{run_id}`
- `GET`
- 响应：
```json
{"run_id":"...","has_checkpoint":true}
```

## 9. 模块化与扩展（K8s 风格）
- 工具和 Agent 通过配置启用，可在 `internal/hub` 中集中构建。
- MCP 工具、外部 Agent 通过配置即插即用。
- CheckPointStore 与 Registry 可替换为持久化实现（Etcd/Consul/DB）。

## 10. 测试策略
- 单元测试：覆盖 `Hub` 的 interrupt/resume、MCP 初始化。
- E2E 测试：模拟 LLM + MCP 服务器，走完整 `/v1/chat` 流程。

## 11. 运行与部署建议
- 多进程：Registry、Hub、LLM、MCP 独立进程。
- 扩展：可在生产环境替换 mock LLM/MCP。
- 监控：在事件流中接入 trace/log/metrics。

## 12. 结论
该设计以 Eino ADK 为核心，保留 transfer/interrupt 语义并接入 MCP 工具与外部 Agent。通过配置化与注册中心机制实现企业级 Agent Hub，能够承接多领域专家 Agent 与 SOP 工作流。

## 13. 实现细节（代码结构）
### 13.1 目录结构
- `cmd/hub`: Hub HTTP 服务入口，负责 API 路由与事件流输出。
- `cmd/registry`: 轻量 Registry 服务，提供 AgentCard 注册与发现。
- `cmd/mock_llm`: OpenAI 兼容 mock LLM，支持 tool_calls/transfer。
- `cmd/mock_mcp`: MCP SSE mock server，提供 `server_metrics` 工具。
- `internal/hub`: Eino ADK 组装层（Supervisor/SubAgents/Tools/Checkpoint）。
- `internal/registry`: Registry 实现（TTL + Heartbeat）。
- `internal/e2e`: 端到端测试。

### 13.2 Hub 组装逻辑
1. `internal/hub/config.go` 读取并补全配置。
2. `internal/hub/hub.go` 负责：
   - 创建 OpenAI ChatModel（Eino ext）。
   - 连接 MCP SSE client，拉取工具列表。
   - 构建 Supervisor 与 SubAgents（Eino ADK）。
   - 使用 `adk.Runner` + `CheckPointStore` 运行与恢复。
3. `internal/hub/tools.go` 实现 `request_approval` 工具并触发 interrupt。

### 13.3 事件封装
`internal/hub/event.go` 将 `adk.AgentEvent` 映射为对外可序列化事件：
- `agent_name`
- `run_path`
- `output.message`
- `action`（transfer/interrupt 等）

## 14. 运行时流程（实现视角）
1. Hub `/v1/chat` 调用 `Runner.Query`。
2. `Runner` 产生 `AgentEvent` 流并写入 CheckPointStore。
3. Hub 汇聚事件，提取最终 `assistant` 消息作为输出。
4. 如触发 interrupt，返回 `interrupts`，由客户端带 `target_id` 再次 `/v1/workflow/resume`。

## 15. 外部 Agent 接入实现
### 15.1 OpenAI API 兼容 Agent
- Registry 发现的 AgentCard 中 `protocol=openai`。
- Hub 将其封装为 `ChatModelAgent`，BaseURL 指向外部 endpoint。

### 15.2 Python Tool Agent
- 推荐实现方式：
  1) Python 侧提供 MCP SSE server。
  2) Hub 配置 MCP SSE URL，即可加载工具。
  3) 工具作为 SubAgent 的工具链，保持统一调用与审计。

## 16. SOP / Workflow 扩展（建议实现）
- 当前版本使用 Supervisor 动态 transfer 调度。
- 后续可引入 Eino `compose.Graph`：
  - 将 SOP 编译为图，支持 DAG/循环/条件分支。
  - Graph 节点可挂载工具或 Agent。
  - 图级别 CheckPointStore 统一中断与恢复。

## 17. 运行脚本与多进程要求
`scripts/run_all.sh` 将 Registry、LLM、MCP、Hub 作为独立进程运行，并提供：
- 端口占用检测与 fail-fast。
- 健康检查等待。
- 运行时配置动态生成（`configs/hub.runtime.json`）。

## 18. 可观测与运维建议
- 事件流：Hub 输出的 `events`/`interrupts` 可直接上报日志或 trace。
- 指标：建议增加 `Runner` 事件统计与调用耗时。
- 注册中心：可替换为持久化存储（etcd/consul）以支持多实例。

## 19. 限制与后续计划
- mock LLM 返回固定文本，未覆盖复杂推理逻辑。
- 当前 Hub 只提供基础 HTTP API，未集成 gRPC。
- 后续可引入：
  - 权限校验（policy/guard）
  - SOP 图编排（compose graph）
  - 多租户与隔离策略
