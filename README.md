# 🦊 Sock Wallet Backend

> **Polygon Amoy 테스트넷 기반 모바일 지갑 백엔드**  
Go + Rust FFI 기반으로 구현된 경량 지갑 서버입니다.

---

## ✨ 주요 기능

- ✅ **BIP39 니모닉 생성**
- ✅ **지갑 주소 및 개인키 생성**
- ✅ **잔액 조회 (Polygon Amoy)**
- ✅ **트랜잭션 전송**
- ✅ **트랜잭션 내역 조회 (Moralis API)**
- ✅ **QR 코드 생성**
- ✅ **최근 송금 주소 저장 (SQLite)**
- ✅ **비밀번호 저장 및 검증 (SQLite + Bcrypt)**
- ✅ **송금 가능 여부 판단 (가스비 포함)**

---

## 🚀 실행 가이드

### 1. 🧱 의존성 설치

#### 필수 설치
| 항목     | 설명                                    | 설치 방법 |
|----------|--------------------------------------|-----------|
| Go       | 백엔드 서버                          | https://go.dev/doc/install            |
| Rust     | 지갑 로직 (FFI용)                    | `curl https://sh.rustup.rs -sSf | sh` |
| SQLite3  | 비밀번호, 최근 주소 저장용              | macOS: `brew install sqlite3`         |
| Moralis  | 트랜잭션/잔액 조회용 API Key 필요       | https://admin.moralis.io              |  

---

### 2. 🔐 환경 변수 설정

`.env` 또는 시스템에 다음 환경 변수 등록:

```bash
export MORALIS_API_KEY=your_moralis_api_key
export ETHERSCAN_API_KEY=your_etherscan_api_key

---

### 3. 🛠 Rust 라이브러리 빌드

Rust로 작성한 FFI 함수들을 빌드하여 정적 라이브러리로 생성합니다.

```bash
cd rust-wallet
cargo build --release
cp target/release/librust_wallet.a ../go-server/
```

4. 📦 Go 모듈 정리 및 의존성 설치
Go 모듈 초기화 및 필요한 패키지를 정리합니다.

```
cd ../go-server
go mod tidy
```
go.mod, go.sum 파일이 프로젝트 루트에 있어야 합니다.

5. 🏃 서버 실행
Go 서버를 실행합니다.
```
go run main.go
```
정상 실행 시 다음과 같은 메시지를 확인할 수 있습니다:
```
🚀 Server running at http://localhost:8080
```
