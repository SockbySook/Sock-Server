#include <stdarg.h>
#include <stdint.h>
#include <stdlib.h>
#include <stdbool.h>
#include <new>

extern "C" {

/// ✅ 24개 단어 니모닉을 생성
char *generate_mnemonic();

/// ✅ 새로운 지갑 주소 생성
char *generate_address();

/// ✅ 특정 주소의 잔액을 조회
char *get_balance_by_address(const char *address);

/// ✅ 트랜잭션 전송 후 트랜잭션 해시 반환
char *send_transaction(const char *to, const char *amount, const char *private_key);

/// ✅ FFI 메모리 해제 함수
void free_string(char *s);

/// ✅ Moralis를 통한 트랜잭션 내역 조회
char *get_transaction_history(const char *address);

/// ✅ 니모닉으로 지갑 복구 (주소와 개인키 반환)
char *recover_wallet_from_mnemonic(const char *mnemonic);

/// ✅ 니모닉 구문 유효성 검증
bool verify_mnemonic(const char *mnemonic);

/// ✅ 송금 가능 여부 확인 (true/false)
bool check_sendable(const char *to, const char *amount, const char *private_key);

/// ✅ 이더스캔 API를 통한 가스비 정보 조회
char *get_gas_price();

/// ✅ 현재 네트워크 정보 조회
char *get_network_info();

/// ✅ 송금 가능 여부 상세 확인 (잔액/가스 포함 JSON 반환)
char *check_sendable_detailed(const char *to, const char *amount, const char *private_key);

/// ✅ 내 지갑 주소 반환
char *get_address_from_private_key(const char *pk_ptr);

/// ✅ private key 생성
char *generate_private_key();

}  // extern "C"
