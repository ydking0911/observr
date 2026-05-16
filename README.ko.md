<p align="right">
  <a href="./README.md">English</a> | <strong>한국어</strong>
</p>

<p align="center">
  <br/>
  <img src="./docs/images/observr.png" width="280" alt="observr">
  <br/>
</p>

<p align="center">
  <strong>AI 에이전트가 그 행동을 왜 했는지 알고 싶다면.</strong>
  <br/>
  <sub>AI 에이전트를 위한 감사 추적 및 인과 귀속 도구 — 한 줄로 계측</sub>
</p>

<p align="center">
  <a href="https://github.com/ydking0911/observr/actions/workflows/ci.yml"><img src="https://github.com/ydking0911/observr/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pypi.org/project/observr/"><img src="https://img.shields.io/pypi/v/observr?color=blue" alt="PyPI"></a>
  <a href="https://www.npmjs.com/package/@ydking0911/observr"><img src="https://img.shields.io/npm/v/@ydking0911/observr" alt="npm"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/ydking0911/observr" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License"></a>
</p>

<p align="center">
  <a href="#문제-상황">문제</a> ·
  <a href="#왜-observr인가">이유</a> ·
  <a href="#핵심-개념">개념</a> ·
  <a href="#시작하기">시작하기</a> ·
  <a href="#감사-로그-쿼리">쿼리</a> ·
  <a href="#로드맵">로드맵</a>
</p>

```python
import observr
observr.init(service="my-agent")
# 이제 이 에이전트의 모든 행동이 기록되고 추적됩니다.
```

---

## 문제 상황

- AI 에이전트가 예상치 못한 행동을 했는데, **어떤 결정이 원인인지** 모르겠다
- 에이전트 3개가 연결되어 있는데, **어디서 실패가 시작됐는지** 역추적할 수 없다
- Claude Code나 Cursor가 tool call을 했는데, **나중에 다시 확인**하고 싶다
- 에이전트가 오류를 냈는데, **어떤 선행 행동이 원인인지** 알고 싶다

observr는 이 질문들에 답하는 도구입니다.

---

## 왜 observr인가?

|  | 직접 로깅 구현 | Datadog / Grafana | **observr** |
|--|:------------:|:-----------------:|:-----------:|
| 에이전트 인과 체인 | 직접 구현 | ✗ | **자동** |
| 결정 역추적 | 직접 구현 | ✗ | **내장** |
| 행동 패턴 감지 | 직접 구현 | 일부 | **내장** |
| 설치 복잡도 | 높음 | 매우 높음 | **1줄** |
| 로컬 / 온프레미스 | 직접 구현 | 유료 | **기본** |
| AI 에이전트 친화 | ✗ | ✗ | **설계됨** |
| 비용 | 개발 시간 | 비쌈 | **무료 오픈소스** |

---

## 핵심 개념

<details>
<summary><strong>인과 귀속 — 모든 결과를 근본 원인까지 역추적</strong></summary>

모든 span은 자신을 트리거한 부모 행동을 가리키는 `parent_span_id`를 가질 수 있습니다. observr는 이를 바탕으로 전체 결정 트리를 재구성합니다.

```python
with observr.get_client().span("agent.decide") as parent:
    with observr.get_client().span("tool.call", parent_span_id=parent.span_id, tool="web_search") as child:
        results = web_search("relevant context")
        child.set_attribute("result_count", len(results))
```

```
trace_id: 4f2a1b3c
├── agent.decide   (a1b2)
│   ├── tool.call  (c3d4, parent: a1b2)  ← agent.decide가 트리거
│   └── db.query   (g7h8, parent: a1b2)
```

```ts
// Node.js
const parent = new Span("agent.decide", transport);
await parent.run(async (p) => {
  const child = new Span("tool.call", transport, { tool: "web_search" }, p.spanId);
  await child.run(async () => { /* 인과적으로 연결됨 */ });
});
```

</details>

<details>
<summary><strong>행동 패턴 — 노이즈가 아닌 신호</strong></summary>

observr는 이벤트 메시지를 정규화해 유사한 것들을 같은 fingerprint로 묶습니다.

`"user abc123 결제 실패"`와 `"user xyz789 결제 실패"`는 같은 패턴으로 집계됩니다 — 개별 로그가 아닌 **시간대별 빈도**로 파악할 수 있습니다.

</details>

<details>
<summary><strong>감사 로그 — 로컬, 쿼리 가능, 영구 저장</strong></summary>

모든 이벤트는 로컬 SQLite(WAL 모드)에 타임스탬프, 서비스 정보, 구조화된 속성과 함께 저장됩니다.

쿼리 가능 필드: 레벨 · 서비스 · trace ID · 시간 범위 · HTTP path

</details>

---

## 시작하기

### 1. 수집기 실행

**macOS / Linux**
```bash
curl -sSL https://raw.githubusercontent.com/ydking0911/observr/main/scripts/install.sh | sh
observrd   # → http://localhost:7676
```

**Homebrew**
```bash
brew tap ydking0911/observr && brew install observr
```

**go install**
```bash
go install github.com/ydking0911/observr/server/cmd/observrd@latest
```

### 2. SDK 설치

```bash
pip install observr               # Python
npm install @ydking0911/observr   # Node.js
```

### 3. 에이전트 계측

**Python — FastAPI / Flask / Django**
```python
import observr
observr.init(service="my-agent")  # 프레임워크 자동 감지
```

**Node.js — Express**
```js
const { init } = require('@ydking0911/observr')
init({ service: 'my-agent' })
```

**에이전트 행동 단위로 수동 span:**
```python
with observr.get_client().span("db.query", table="users") as span:
    rows = db.execute("SELECT ...")
    span.set_attribute("row_count", len(rows))
```

**로그는 자동으로 수집됩니다:**
```python
import logging
logger = logging.getLogger(__name__)
logger.error("Payment failed", extra={"user_id": "u_123", "amount": 9900})
```

---

## 감사 로그 쿼리

```bash
# 최근 오류 (JSON)
observrd query --level error --last 100 --format json

# 특정 에이전트의 모든 행동
observrd query --service my-agent --last 500 --format json

# 전체 결정 트리 추적
observrd query --trace-id 4f2a1b3c

# 검토용 내보내기
observrd query --level error --last 10000 --format csv > audit.csv

# 사람이 읽기 좋은 테이블
observrd query --format text
```

**예시 — AI 에이전트가 자신의 감사 로그를 쿼리:**
```
User: 지난 1시간 동안 에이전트가 어떤 오류를 냈어?
Claude: 감사 로그 확인할게요...
$ observrd query --service my-agent --level error --last 200 --format json
→ 오류 3건 모두 span "tool.call" → parent "agent.decide" at 14:32:01
→ 근본 원인: agent.decide가 잘못된 입력을 tool.call에 전달
```

---

## 알림 설정

에러가 임계값을 초과하면 Slack / Discord 알림:

```bash
observrd start \
  --alert-slack-url   https://hooks.slack.com/services/... \
  --alert-discord-url https://discord.com/api/webhooks/... \
  --alert-level       error \
  --alert-threshold   5 \
  --alert-window      60s \
  --alert-cooldown    5m
```

---

## 이벤트 스키마

```json
{
  "id":             "evt_1711234567890",
  "trace_id":       "4f2a1b3c8e9d0f1a",
  "span_id":        "a1b2c3d4",
  "parent_span_id": "9f8e7d6c",
  "service":        "my-agent",
  "timestamp":      "2026-03-24T12:34:56.789Z",
  "type":           "span",
  "level":          "error",
  "duration_ms":    3241.5,
  "message":        "tool.call failed",
  "attributes": {
    "tool":  "web_search",
    "error": "timeout after 3000ms"
  }
}
```

`parent_span_id`는 span을 인과적 부모와 연결해 중첩된 에이전트 행동 전체의 결정 트리 재구성을 가능하게 합니다.

---

## 로드맵

| 버전 | 상태 | 기능 |
|------|:----:|------|
| **v0.1** | ✅ | Python SDK · Go collector · React 대시보드 · CLI · CI/CD |
| **v0.2** | ✅ | Node.js SDK · PyPI 배포 · npm 배포 |
| **v0.3** | ✅ | Slack/Discord 알림 · 이벤트 보존 정책(TTL) · JSON/CSV 내보내기 |
| **v0.4** | ✅ | 인과 귀속 (`parent_span_id`) · 행동 패턴 감지 · Fastify 지원 |
| **v0.5** | 📋 | 감사 리포트 생성 · 인과 체인 내보내기 · 멀티 에이전트 트레이싱 |
| **v0.6** | 📋 | Go SDK · 정책 규칙 엔진 · 온체인 앵커링 |

---

## 개발 참여

```bash
make build          # 전체 빌드 (대시보드 바이너리 포함)
make dev-server     # Go 서버 :7676
make dev-dashboard  # Vite 개발 서버 :5173 (:7676 프록시)
make test           # Go + Python + Node.js
make test-e2e       # 전체 E2E 테스트
make lint
```

[CONTRIBUTING.md](CONTRIBUTING.md)에서 기여 방법을 확인하세요.

---

<p align="center">
  <sub>MIT License · <a href="LICENSE">LICENSE</a></sub>
</p>
