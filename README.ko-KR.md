<p align="center">
  <img src="https://img.shields.io/badge/amqcli-ActiveMQ%20TUI%20Client-4A90D9?style=for-the-badge&logo=apache-activemq&logoColor=white" alt="amqcli banner">
</p>

<h1 align="center">💫 amqcli — ActiveMQ TUI Client</h1>

<p align="center">
  <b>터미널 환경에서 Apache ActiveMQ 브로커를 손쉽게 관리하세요.</b><br>
  메시지 조회, 큐 관리, 통계 모니터링 기능이 세련된 TUI 환경에서 모두 제공됩니다.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version">
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License: MIT">
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=for-the-badge" alt="Platform">
  <img src="https://img.shields.io/badge/Arch-amd64%20%7C%20arm64-blueviolet?style=for-the-badge" alt="Architecture">
  <img src="https://img.shields.io/badge/CGO-Disabled-orange?style=for-the-badge" alt="CGO Disabled">
  <a href="README.md"><img src="https://img.shields.io/badge/Lang-English-red?style=for-the-badge" alt="English"></a>
</p>

---

## 프로젝트 개요

**amqcli**는 [Apache ActiveMQ](https://activemq.apache.org/) 브로커를 손쉽게 관리하고 모니터링하기 위해 설계된 **TUI(Terminal User Interface)** 도구입니다. Go 언어와 [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) 프레임워크 기반으로 개발되어, 웹 관리자 콘솔을 대체할 수 있는 빠르고 키보드 친화적인 환경을 제공합니다.

Jolokia(JMX) 및 AMQP/STOMP 프로토콜과 직접 연동되므로, 터미널을 벗어나지 않고도 강력한 ActiveMQ 관리 기능을 모두 경험할 수 있습니다.

```mermaid
flowchart LR
    %% Styles
    classDef default fill:#f9f9f9,stroke:#333,stroke-width:1px,color:#333
    classDef highlight fill:#e1f5fe,stroke:#0288d1,stroke-width:2px,color:#01579b,font-weight:bold
    classDef engine fill:#fff3e0,stroke:#f57c00,stroke-width:2px,color:#e65100,font-weight:bold
    classDef amqcli fill:#e8eaf6,stroke:#3f51b5,stroke-width:2px,color:#1a237e,font-weight:bold

    A["💻 터미널 사용자<br/>(키보드 조작)"]:::highlight -->|Interact| B("⚡ amqcli<br/>(TUI Engine)"):::amqcli
    B -->|Jolokia / JMX| C{"📦 Apache ActiveMQ"}:::engine
    B -->|AMQP / STOMP| C
    C -.->|브로커 통계<br/>및 메시지 반환| B
```

---

## 주요 기능

<table>
<tr><td><b>TUI 기반 환경</b></td><td>Bubbletea로 구동되는 최신 크로스 플랫폼 터미널 인터페이스를 통해 부드러운 네비게이션과 Catppuccin 컬러 테마를 제공합니다. (구형 VT100 자동 Fallback 지원)</td></tr>
<tr><td><b>큐(Queue) 관리</b></td><td>모든 큐의 목록과 소비자(Consumer) 수, Enqueue/Dequeue 메트릭을 실시간으로 확인하고 큐 생성, 퍼지(Purge), 삭제 작업을 지원합니다.</td></tr>
<tr><td><b>메시지 조회</b></td><td>UI 환경 내에서 큐 내부의 메시지를 직접 조회하고, 메시지 속성과 페이로드(Payload), Correlation ID 등을 상세히 확인할 수 있습니다.</td></tr>
<tr><td><b>메시지 전송</b></td><td>AMQP 1.0 또는 STOMP 프로토콜을 통해 타겟 큐로 메시지를 즉시 전송할 수 있습니다.</td></tr>
<tr><td><b>브로커 통계</b></td><td>큐 리스트 화면에서 단축키(U)로 시스템 리소스(CPU, 메모리, 디스크) 사용량 및 스토어 크기 등의 상태를 토글하여 확인할 수 있습니다.</td></tr>
<tr><td><b>고급 삭제 기능</b></td><td>한 번에 여러 개의 메시지를 일괄 삭제하거나, 특정 시간 이전에 생성된 메시지를 필터링하여 일괄 삭제할 수 있습니다.</td></tr>
<tr><td><b>연결(Connection) 정보</b></td><td>활성화된 클라이언트 연결 정보, 리모트 주소, 유지 시간(Uptime) 등을 조회하여 브로커의 상태를 점검할 수 있습니다.</td></tr>
</table>

---

## 시작하기

### 1. 설정 파일 (`config.yml`) 구성

`amqcli`는 환경(예: `dev`, `prod`) 단위로 브로커를 관리하기 위해 `config.yml` 설정 파일을 사용합니다. 실행 파일과 동일한 경로에 파일을 생성하세요.

```yaml
# config.yml 예시
refresh_interval: 1s
encoding: utf-8
environments:
  dev:
    protocol: "stomp"  # 또는 "amqp"
    host: "${MQ_HOST:-127.0.0.1}"
    stomp_port: "61613" # 선택 사항 (기본값: 61613)
    web_port: "8161"    # 선택 사항 (기본값: 8161)
    username: "${MQ_USER:-admin}"
    password: "${MQ_PASS:-admin}"
  prod:
    protocol: "amqp"
    host: "10.0.0.5"
    amqp_port: "5672"   # 선택 사항 (기본값: 5672)
    web_port: "8161"    # 선택 사항 (기본값: 8161)
    username: "admin"
    password: "prod-password"
```

### 2. amqcli 실행

```bash
# 기본 환경설정('dev')으로 실행할 경우
./amqcli

# config.yml 내에 정의된 특정 환경('prod')으로 실행할 경우
./amqcli -env prod
```

### 3. 주요 단축키

- `↑` / `↓` : 상하 항목 이동
- `Enter` : 항목 선택 / 큐 내부 메시지 조회 / 메시지 상세 보기
- `C` : 새 큐 생성 (Create)
- `S` : 메시지 전송 (Send)
- `P` : 큐 비우기 (Purge)
- `D` : 큐 삭제 / 개별 메시지 삭제 (Delete)
- `I` : 큐 통계 및 상세 정보 확인 (Info)
- `N` : 활성 연결(Connection) 목록 조회
- `U` : 시스템 Usage 통계(Memory/Disk/CPU 등) 토글 표시
- `F3` / `Ctrl+F` : 메시지 검색 (Search)
- `Esc` : 뒤로 가기
- `q` / `Ctrl+C` : 애플리케이션 종료

---

## 설치 방법

다음 방법 중 하나를 선택하여 `amqcli`를 설치할 수 있습니다. `amqcli` 바이너리는 정적 링크된 실행 파일(`CGO_ENABLED=0`)로 배포되므로 별도의 외부 종속성 없이 독립적으로 실행됩니다.

### 1. Homebrew (macOS / Linux)
커스텀 탭을 통해 Homebrew로 매우 간단하게 설치 및 업그레이드할 수 있습니다:
```bash
brew tap xvlet/amqcli
brew install amqcli
```

### 2. 간편 설치 스크립트 (Quick Install)
운영체제에 맞는 설치 스크립트를 사용하여 최신 릴리즈를 가장 쉽게 설치하는 방법입니다.

**macOS / Linux (Shell)**
```bash
curl -fsSL https://raw.githubusercontent.com/xvlet/amqcli/master/install.sh | sh
```

**Windows (PowerShell)**
```powershell
powershell -ExecutionPolicy Bypass -c "irm https://raw.githubusercontent.com/xvlet/amqcli/master/install.ps1 | iex"
```

### 3. Go 명령어 사용 (go install)
Go(1.25 이상)가 설치된 환경이라면 `go install` 명령어로 쉽게 설치할 수 있습니다:
```bash
go install github.com/xvlet/amqcli/cmd/amqcli@latest
```

### 4. 컴파일된 바이너리 다운로드
Go가 설치되어 있지 않고 실행 파일만 바로 사용하고 싶다면, 최신 릴리즈에서 바이너리를 직접 다운로드하세요.
- [Releases 페이지에서 바이너리 다운로드](https://github.com/xvlet/amqcli/releases)

다운로드 후 압축을 풀고 실행 권한을 부여한 뒤 실행합니다:
```bash
tar -xzf amqcli_linux_amd64.tar.gz
chmod +x amqcli
./amqcli -h
```

### 5. Docker (GHCR)
GitHub Container Registry(GHCR)를 통해 배포되는 공식 Docker 이미지를 이용해 바로 실행할 수 있습니다:
```bash
docker run -it --rm -v $(pwd)/config.yml:/app/config.yml ghcr.io/xvlet/amqcli:latest
```

---

## 요구 사항

`amqcli`는 단일 바이너리로 정적 컴파일(Statically compiled) 되므로, 실행 시 **외부 라이브러리 의존성을 요구하지 않습니다**.

| 도구 | 목적 |
|------|---------|
| [Apache ActiveMQ](https://activemq.apache.org/) | 대상 메시지 브로커입니다. 내부적으로 Jolokia(REST API) 및 AMQP/STOMP 포트 접근이 허용되어야 정상 작동합니다. |
