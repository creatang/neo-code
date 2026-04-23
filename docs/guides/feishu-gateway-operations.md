# Feishu Gateway 适配器运维手册

本文面向运维与值班同学，覆盖 `neocode feishu-adapter` 的启动、观测、故障处置、兼容回归和回滚策略。

## 1. 启动方式

```bash
neocode feishu-adapter \
  --listen 127.0.0.1:19091 \
  --gateway-ws ws://127.0.0.1:8080/ws \
  --gateway-token-file ~/.neocode/auth.json \
  --signing-secret "$FEISHU_SIGNING_SECRET" \
  --route-file /etc/neocode/feishu-routes.json \
  --dedupe-store-file /var/lib/neocode/feishu-dedupe.json \
  --alert-webhook-url "$FEISHU_ALERT_WEBHOOK"
```

说明：
1. 适配器应部署在内网，Gateway 不直接暴露公网。
2. `--dedupe-store-file` 用于重启后保留 `event_id/run_id` 去重窗口。
3. `--alert-webhook-url` 用于发送关键告警。

## 2. 健康与指标

暴露端点：
1. `GET /healthz`：整体状态 + 指标快照。
2. `GET /metrics`：JSON 指标详情。

关键指标：
1. `runs_accepted/runs_completed/runs_failed/runs_canceled`
2. `watchdog_timeouts`
3. `connection_reconnects`
4. `auth_failures`
5. `event_duplicates`
6. `permissions_pending`
7. `inflight_runs`

告警建议：
1. `watchdog_timeouts` 增长：终态超时。
2. `connection_reconnects` 连续增长：连接抖动。
3. `auth_failures` 突增：鉴权异常。
4. `inflight_runs` 长期高位：事件积压。

## 3. 权限审批卡片闭环

适配器会在收到 `permission_requested` 时发送飞书交互卡片（含三种决策）：
1. `allow_once`
2. `allow_session`
3. `reject`

回调可通过 `POST /feishu/actions/permission` 提交，支持两类载荷：
1. 直传格式：`{"request_id":"...","decision":"allow_once"}`
2. 飞书卡片 action 回调格式（自动提取 `action.value.request_id/decision`）。

## 4. 故障处理

### 4.1 正常链路验证
1. 飞书消息进入 `/feishu/events` 返回 `code=0`。
2. 适配器执行 `authenticate -> bindStream -> run`。
3. 收到 `gateway.event` 终态（`run_done` 或 `run_error`）并回写。

### 4.2 异常链路
1. `unauthorized`：检查 `--gateway-token-file` 与 token 内容。
2. `invalid_frame / missing_required_field / invalid_action`：检查请求体。
3. `access_denied`：检查 Gateway ACL。
4. `timeout / internal_error`：检查 Runtime 负载与下游延迟。

### 4.3 恢复链路
1. 连接断开后，客户端自动重连并重新认证。
2. 重连后回放历史 `bindStream` 绑定。
3. `watchdog` 超时会自动 `cancel(run_id)` 并回写终态。

## 5. 多租户与回滚开关

路由文件支持按租户/群组策略：

```json
{
  "default": {
    "enabled": true,
    "streaming": true,
    "workdir": "/srv/workspaces/default"
  },
  "tenants": {
    "tenant-a": {
      "enabled": true,
      "streaming": false,
      "workdir": "/srv/workspaces/tenant-a"
    }
  },
  "chats": {
    "tenant-a:oc_xxx": {
      "enabled": true,
      "streaming": true
    }
  }
}
```

回滚开关：
1. 按租户关闭：`tenants.<tenant>.enabled=false`
2. 按群关闭：`chats.<tenant:chat>.enabled=false`
3. 降级为仅最终回写：`streaming=false`

## 6. 双版本兼容回归

上线前执行双版本回归（当前版本 + 目标版本）。

### 6.1 CLI 模式

```bash
neocode feishu-adapter \
  --compat-check \
  --gateway-token-file ~/.neocode/auth.json \
  --compat-targets "ws://gw-v1:8080/ws,ws://gw-v2:8080/ws"
```

### 6.2 脚本模式

```bash
GATEWAY_TARGETS="ws://gw-v1:8080/ws,ws://gw-v2:8080/ws" \
./scripts/feishu_gateway_compat_check.sh
```

检查项：
1. `gateway.authenticate`
2. `gateway.ping`
3. `gateway.listSessions`

## 7. 故障演练脚本

参考：`scripts/feishu_gateway_drill.sh`
