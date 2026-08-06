const translations = {
  en: {
    navCapabilities: "Capabilities", navWorkflow: "Workflow", navArchitecture: "Architecture", viewGithub: "View on GitHub",
    eyebrow: "PROVIDER-NEUTRAL MULTI-AGENT OPERATIONS", heroTitle: "Turn production signals into <em>explainable answers.</em>", heroLede: "Knsight coordinates specialized agents, tools, and operational context to investigate incidents—while keeping every step visible and reviewable.", getStarted: "Get started", exploreCode: "Explore the code", proofCore: "Fast, portable core", proofTools: "Standardized tools", proofUi: "Interactive workspace", live: "LIVE", incidentTitle: "Checkout latency above SLO", incidentMeta: "payment-api · p99 2.4s · 14:32 UTC", investigating: "Investigating", nodeLead: "Lead agent", nodeScope: "Scopes incident", nodeMetrics: "Metrics", nodeSpike: "Finds DB spike", nodeLogs: "Logs", nodeErrors: "Correlates errors", nodeCause: "Root cause", nodePool: "Connection pool saturation", confidence: "confidence", findingLabel: "PRIMARY FINDING", findingText: "Database connections exhausted after deployment",
    whyKicker: "WHY KNSIGHT", whyTitle: "Incident response has enough data. What it needs is a clear line of reasoning.", whyBody: "Knsight turns scattered metrics, logs, tools, and domain knowledge into a coordinated investigation. Operators can follow the evidence, inspect agent decisions, and reuse the outcome.",
    capKicker: "CAPABILITIES", capTitle: "One investigation surface.<br />Four compounding capabilities.", capBody: "Built for teams that need flexible integrations without losing control, context, or explainability.", cap1Title: "Diagnose", cap1Body: "Receive an incident in natural language, extract its scope, and turn symptoms into testable hypotheses.", cap1Tag: "Intent + context", cap2Title: "Connect", cap2Body: "Discover services and call MCP-compatible tools across monitoring, logs, traces, and internal systems.", cap2Tag: "Registry + MCP", cap3Title: "Reason", cap3Body: "Coordinate specialized sub-agents, verify evidence in parallel, and preserve the full analysis trail.", cap3Tag: "Multi-agent orchestration", cap4Title: "Learn", cap4Body: "Persist sessions and reports so resolved incidents become reusable operational knowledge.", cap4Tag: "Sessions + memory",
    flowKicker: "HOW IT WORKS", flowTitle: "From alert to accountable answer.", flowBody: "A structured loop keeps the investigation focused while letting agents explore independent evidence paths.", step1Title: "Frame the incident", step1Body: "Normalize the request, identify affected services, and establish time and safety boundaries.", step1State: "CONTEXT", step2Title: "Delegate the evidence", step2Body: "Select specialized agents and tools, then investigate metrics, logs, dependencies, and hosts in parallel.", step2State: "PARALLEL", step3Title: "Test the hypotheses", step3Body: "Compare signals, reject weak explanations, and converge on the most supported failure path.", step3State: "VERIFY", step4Title: "Explain and retain", step4Body: "Produce a structured report with evidence, confidence, and next actions; persist it for reuse.", step4State: "REPORT",
    agentKicker: "AGENT SYSTEM", agentTitle: "One supervisor coordinates.<br />Specialists go deep.", agentBody: "Knsight uses hierarchical delegation: the top-level supervisor owns the plan, domain sub-agents own each investigation stage, and tool agents connect the reasoning loop to real systems.", tierOrchestration: "ORCHESTRATION", supervisorType: "SUPERVISOR", supervisorRole: "Plans, delegates, verifies, and synthesizes", tierSpecialists: "SPECIALISTS", subAgentType: "SUB-AGENT", inspectRole: "Collects and correlates evidence", visionRole: "Turns metrics into visual insight", summaryRole: "Produces the structured RCA report", tierTools: "TOOLS & SYSTEMS", toolAgentType: "TOOL AGENT · EXAMPLE", cloudRole: "Runs host and service diagnostics", targetTools: "Operational tools", targetHosts: "Services & hosts", agentNote: "Typical deep-inspection path shown. Agent names and tool connections are configuration-driven.",
    archKicker: "ARCHITECTURE", archTitle: "Open at every boundary.", archBody: "Use the model endpoint, storage layer, and operational tools that fit your environment. Knsight keeps provider-specific credentials and private knowledge outside the framework.", arch1: "OpenAI-compatible model endpoints", arch2: "SQLite by default, optional Redis persistence", arch3: "Sandboxed tool execution and web access controls", arch4: "JWT and identity-token authentication modes", layer1: "Operator + Web UI", layer2: "Knsight Agent Hub", layer3a: "Models", layer3b: "MCP tools", layer3c: "Storage", providerNeutral: "Provider-neutral by design",
    startKicker: "QUICK START", startTitle: "Explore locally with the included mock services.", startBody: "Run the registry, model, and MCP mocks, then start the hub. No production credentials are needed for the demo path.", readGuide: "Read the full guide", copy: "Copy", copied: "Copied",
    ctaKicker: "BUILD WITH KNSIGHT", ctaTitle: "Give your incident response a reasoning layer.", ctaBody: "Inspect the architecture, run the demo, and shape the agent system around your operational environment.", openRepository: "Open repository", shareFeedback: "Share feedback", footerText: "Explainable multi-agent operations, designed to be inspected and extended."
  },
  zh: {
    navCapabilities: "核心能力", navWorkflow: "工作流程", navArchitecture: "系统架构", viewGithub: "查看 GitHub",
    eyebrow: "供应商中立的 MULTI-AGENT 运维平台", heroTitle: "让生产信号沉淀为<em>可解释的答案。</em>", heroLede: "Knsight 协同专业 Agent、工具与运维上下文完成故障调查，同时让每一步分析都可见、可查、可复核。", getStarted: "开始使用", exploreCode: "浏览代码", proofCore: "快速、可移植的核心", proofTools: "标准化工具连接", proofUi: "交互式分析工作台", live: "实时", incidentTitle: "结算链路延迟超出 SLO", incidentMeta: "payment-api · p99 2.4s · 14:32 UTC", investigating: "分析中", nodeLead: "主 Agent", nodeScope: "界定故障范围", nodeMetrics: "指标 Agent", nodeSpike: "发现数据库峰值", nodeLogs: "日志 Agent", nodeErrors: "关联异常日志", nodeCause: "根因", nodePool: "连接池耗尽", confidence: "置信度", findingLabel: "主要结论", findingText: "发布后数据库连接资源耗尽",
    whyKicker: "为什么选择 KNSIGHT", whyTitle: "故障响应不缺数据，缺的是一条清晰的推理链。", whyBody: "Knsight 把分散的指标、日志、工具和领域知识组织成一次协同调查。运维人员可以跟随证据、检查 Agent 决策，并复用最终结论。",
    capKicker: "核心能力", capTitle: "一个调查界面，<br />四项持续增强的能力。", capBody: "面向既需要灵活集成，又不能牺牲控制力、上下文和可解释性的团队。", cap1Title: "智能诊断", cap1Body: "用自然语言接收故障，提取影响范围，将表象转化为可验证的诊断假设。", cap1Tag: "意图 + 上下文", cap2Title: "深度连接", cap2Body: "发现服务，并通过 MCP 兼容工具连接监控、日志、链路和内部系统。", cap2Tag: "注册中心 + MCP", cap3Title: "根因推理", cap3Body: "协调专业子 Agent 并行验证证据，同时保留完整的分析轨迹。", cap3Tag: "Multi-Agent 编排", cap4Title: "知识沉淀", cap4Body: "持久化会话和报告，让已解决的故障转化为可复用的运维知识。", cap4Tag: "会话 + 记忆",
    flowKicker: "工作原理", flowTitle: "从告警到可追溯的答案。", flowBody: "结构化循环让调查保持聚焦，同时允许多个 Agent 沿独立证据路径并行探索。", step1Title: "界定故障", step1Body: "规范化请求，识别受影响服务，并明确时间范围与安全边界。", step1State: "上下文", step2Title: "分派证据任务", step2Body: "选择专业 Agent 和工具，并行调查指标、日志、依赖与主机。", step2State: "并行", step3Title: "验证诊断假设", step3Body: "交叉比较信号，排除薄弱解释，收敛到证据最充分的故障路径。", step3State: "验证", step4Title: "解释并沉淀", step4Body: "输出包含证据、置信度和后续建议的结构化报告，并持久化以便复用。", step4State: "报告",
    agentKicker: "AGENT 系统", agentTitle: "一个 Supervisor 统筹，<br />多个专业 Agent 深入分析。", agentBody: "Knsight 使用分层委派：顶层 Supervisor 负责计划，领域子 Agent 负责各调查阶段，Tool Agent 将推理循环连接到真实系统。", tierOrchestration: "编排层", supervisorType: "SUPERVISOR", supervisorRole: "制定计划、分派任务、验证证据并汇总结论", tierSpecialists: "专业子 AGENT", subAgentType: "子 AGENT", inspectRole: "采集并关联诊断证据", visionRole: "将指标转化为可视化洞察", summaryRole: "生成结构化 RCA 报告", tierTools: "工具与系统", toolAgentType: "TOOL AGENT · 示例", cloudRole: "执行主机与服务诊断", targetTools: "运维工具", targetHosts: "服务与主机", agentNote: "图中展示典型深度检查路径；Agent 名称与工具连接均由配置驱动。",
    archKicker: "系统架构", archTitle: "每一层都保持开放。", archBody: "自由选择适合现有环境的模型端点、存储层和运维工具。Knsight 将供应商凭据与私有知识保留在框架之外。", arch1: "兼容 OpenAI API 的模型端点", arch2: "默认 SQLite，可选 Redis 持久化", arch3: "沙箱化工具执行与网络访问控制", arch4: "JWT 与身份令牌认证模式", layer1: "运维人员 + Web 界面", layer2: "Knsight Agent Hub", layer3a: "模型", layer3b: "MCP 工具", layer3c: "存储", providerNeutral: "供应商中立设计",
    startKicker: "快速开始", startTitle: "使用内置 Mock 服务在本地体验。", startBody: "启动注册中心、模型与 MCP Mock，再运行 Hub。演示路径无需任何生产凭据。", readGuide: "阅读完整指南", copy: "复制", copied: "已复制",
    ctaKicker: "开始构建", ctaTitle: "为故障响应加上一层推理能力。", ctaBody: "了解系统架构、运行演示，并围绕你的运维环境构建 Agent 系统。", openRepository: "打开代码仓库", shareFeedback: "反馈建议", footerText: "可检查、可扩展的可解释 Multi-Agent 运维平台。"
  }
};

const toggle = document.querySelector("#language-toggle");
const labels = document.querySelectorAll("[data-language-label]");
const initialLanguage = new URLSearchParams(location.search).get("lang") || localStorage.getItem("knsight-language") || "en";

function setLanguage(language) {
  const selected = translations[language] ? language : "en";
  document.documentElement.lang = selected === "zh" ? "zh-CN" : "en";
  document.querySelectorAll("[data-i18n]").forEach((element) => {
    const value = translations[selected][element.dataset.i18n];
    if (value) element.innerHTML = value;
  });
  labels.forEach((label) => label.classList.toggle("active", label.dataset.languageLabel === selected));
  toggle.setAttribute("aria-pressed", selected === "zh" ? "true" : "false");
  toggle.setAttribute("aria-label", selected === "zh" ? "Switch to English" : "切换为中文");
  localStorage.setItem("knsight-language", selected);
  document.title = selected === "zh" ? "Knsight — 可解释的 AI 故障诊断" : "Knsight — Explainable AI for Production Incidents";
}

toggle.addEventListener("click", () => setLanguage(document.documentElement.lang.startsWith("zh") ? "en" : "zh"));
setLanguage(initialLanguage);

const observer = new IntersectionObserver((entries) => {
  entries.forEach((entry) => { if (entry.isIntersecting) { entry.target.classList.add("visible"); observer.unobserve(entry.target); } });
}, { threshold: 0.12 });
document.querySelectorAll(".reveal").forEach((element) => observer.observe(element));

const copyButton = document.querySelector("#copy-command");
copyButton.addEventListener("click", async () => {
  const command = document.querySelector("#quickstart-code").innerText;
  await navigator.clipboard.writeText(command);
  const language = document.documentElement.lang.startsWith("zh") ? "zh" : "en";
  copyButton.querySelector("span").textContent = translations[language].copied;
  setTimeout(() => { copyButton.querySelector("span").textContent = translations[language].copy; }, 1500);
});

document.querySelector("#year").textContent = new Date().getFullYear();
