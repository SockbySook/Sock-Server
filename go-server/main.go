package main

/*
#cgo darwin LDFLAGS: -framework CoreFoundation -framework SystemConfiguration
#cgo LDFLAGS: -L${SRCDIR}/.. -lrust_wallet -lm -lz -ldl -lpthread
#cgo CFLAGS: -I${SRCDIR}/..
#include <stdlib.h>
#include <stdbool.h>
#include "rust_wallet.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"go-server/db"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"

	//"os"
	"unsafe"

	// 👇 Swagger UI 핸들러 및 문서 임포트
	_ "go-server/docs"

	httpSwagger "github.com/swaggo/http-swagger"
)

// ✅ 리팩토링: 공통 에러 응답 함수
func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// ✅ 리팩토링: 공통 JSON 응답 함수
func respondWithJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

type passwordPayload struct {
	Password string `json:"password"`
}

type MnemonicResponse struct {
	Mnemonic string `json:"mnemonic"`
}

type AddressResponse struct {
	Address string `json:"address"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type BalanceResponse struct {
	Balance string `json:"balance"`
	Symbol  string `json:"symbol"`
	Network string `json:"network"`
}

type RecoverResponse struct {
	Address    string `json:"address"`
	PrivateKey string `json:"privatekey"`
}

type SendableResponse struct {
	CanSend bool `json:"cansend"`
}

type TransactionResponse struct {
	TxHash string `json:"txhash"`
}

type PasswordStatusResponse struct {
	Status string `json:"status"`
}

type RecentAddress struct {
	Address  string `json:"address"`
	LastUsed string `json:"lastused"`
}

type RegisteredWallet struct {
	Address      string `json:"address"`
	Label        string `json:"label"`
	RegisteredAt string `json:"registeredat"`
}

// ✅ 최근 송금 주소를 DB에 저장
func saveRecentAddress(address string) {
	conn := db.GetDB()
	if conn == nil {
		fmt.Println("❗ DB 연결이 초기화되지 않았습니다.")
		return
	}

	stmt := `
	INSERT INTO recent_addresses (address, last_used)
	VALUES (?, ?)
	ON CONFLICT(address) DO UPDATE SET last_used=excluded.last_used;
	`
	_, err := conn.Exec(stmt, address, time.Now())
	if err != nil {
		fmt.Println("❌ 주소 저장 실패:", err)
	} else {
		fmt.Println("📌 최근 송금 주소 저장됨:", address)
	}
}

// ✅ 니모닉 생성
// generateMnemonicHandler godoc
// @Summary     니모닉 생성
// @Description 24개 단어의 니모닉을 생성합니다. 🔁 내부적으로 Rust FFI 사용
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Success     200 {object} MnemonicResponse
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/create [get]
func generateMnemonicHandler(w http.ResponseWriter, r *http.Request) {
	mnemonicPtr := C.generate_mnemonic()
	mnemonic := C.GoString(mnemonicPtr)
	C.free(unsafe.Pointer(mnemonicPtr))

	respondWithJSON(w, map[string]string{"mnemonic": mnemonic})
}

// ✅ 새로운 지갑 주소 생성
// generateAddressHandler godoc
// @Summary     지갑 주소 생성
// @Description 랜덤한 새로운 지갑 주소를 생성합니다. 🔁 내부적으로 Rust FFI 사용
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Success     200 {object} AddressResponse
// @Failure     500 {object} ErrorResponse
// @Router      /wallets/address [get]
func generateAddressHandler(w http.ResponseWriter, r *http.Request) {
	addrPtr := C.generate_address()
	address := C.GoString(addrPtr)
	C.free(unsafe.Pointer(addrPtr))

	respondWithJSON(w, map[string]string{"address": address})
}

// ✅ 현재 지갑 잔액 조회
// getBalanceHandler godoc
// @Summary     잔액 조회
// @Description 지갑 주소의 잔액을 조회합니다. 🔁 내부적으로 Rust FFI 사용
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Param       address query string true "지갑 주소"
// @Success     200 {object} BalanceResponse
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/balance [get]
func getBalanceHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		respondWithError(w, http.StatusBadRequest, "Missing address parameter")
		return
	}

	addrC := C.CString(address)
	defer C.free(unsafe.Pointer(addrC))

	resultPtr := C.get_balance_by_address(addrC)
	defer C.free(unsafe.Pointer(resultPtr))

	balanceJson := C.GoString(resultPtr)
	respondWithJSON(w, json.RawMessage(balanceJson))
}

// ✅ 거래 내역(Moralis API) 가져오기
// getTxHistoryHandler godoc
// @Summary     거래 내역 조회 (Go)
// @Description Moralis API를 통해 거래 내역을 조회합니다. 🔁 내부적으로 Rust FFI 사용
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Param       address query string true "지갑 주소"
// @Success     200 {object} map[string]interface{}
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/history/go [get]
func getTxHistoryHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		respondWithError(w, http.StatusBadRequest, "Missing address parameter")
		return
	}

	apiKey := os.Getenv("MORALIS_API_KEY")
	if apiKey == "" {
		respondWithError(w, http.StatusInternalServerError, "MORALIS_API_KEY is not set on the server")
		return
	}

	url := fmt.Sprintf("https://deep-index.moralis.io/api/v2/%s?chain=amoy", address)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create request")
		return
	}

	req.Header.Set("accept", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Request to Moralis failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read Moralis response", http.StatusInternalServerError)
		return
	}

	if resp.StatusCode != 200 {
		log.Printf("❌ Moralis API error: status=%d, body=%s", resp.StatusCode, body)
		http.Error(w, string(body), resp.StatusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// ✅ FFI 기반 거래 내역 조회
// getHistoryHandler godoc
// @Summary     거래 내역 조회 (Rust)
// @Description Rust FFI를 통해 거래 내역을 조회합니다. 🔁 내부적으로 Rust FFI 사용
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Param       address query string true "지갑 주소"
// @Success     200 {object} map[string]interface{}
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/history/ffi [get]
func getHistoryHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "Missing address parameter", http.StatusBadRequest)
		return
	}

	addressC := C.CString(address)
	defer C.free(unsafe.Pointer(addressC))

	respPtr := C.get_transaction_history(addressC)
	defer C.free(unsafe.Pointer(respPtr))

	historyJSON := C.GoString(respPtr)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(historyJSON))
}

// ✅ 니모닉 기반 지갑 복구
// recoverWalletHandler godoc
// @Summary     지갑 복구
// @Description 니모닉으로 지갑을 복구합니다. 🔁 내부적으로 Rust FFI 사용
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Param       mnemonic query string true "니모닉 24단어"
// @Success     200 {object} RecoverResponse
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/recover [get]
func recoverWalletHandler(w http.ResponseWriter, r *http.Request) {
	mnemonic := r.URL.Query().Get("mnemonic")
	if mnemonic == "" {
		http.Error(w, "Missing mnemonic parameter", http.StatusBadRequest)
		return
	}

	mnemonicC := C.CString(mnemonic)
	defer C.free(unsafe.Pointer(mnemonicC))

	resultPtr := C.recover_wallet_from_mnemonic(mnemonicC)
	defer C.free(unsafe.Pointer(resultPtr))

	result := C.GoString(resultPtr)

	var response map[string]string
	if result == "Invalid mnemonic" {
		response = map[string]string{"error": "Invalid mnemonic"}
	} else {
		response = map[string]string{"wallet": result}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ✅ 니모닉 유효성 검증
// verifyMnemonicHandler godoc
// @Summary     니모닉 유효성 확인
// @Description 입력한 니모닉의 유효성을 검증합니다. 🔁 내부적으로 Rust FFI 사용
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Param       mnemonic query string true "니모닉 24단어"
// @Success     200 {object} map[string]bool
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/verify [get]
func verifyMnemonicHandler(w http.ResponseWriter, r *http.Request) {
	mnemonic := r.URL.Query().Get("mnemonic")
	if mnemonic == "" {
		http.Error(w, "Missing mnemonic", http.StatusBadRequest)
		return
	}

	mnemonicC := C.CString(mnemonic)
	defer C.free(unsafe.Pointer(mnemonicC))

	isValid := C.verify_mnemonic(mnemonicC)

	resp := map[string]bool{"valid": bool(isValid)}
	json.NewEncoder(w).Encode(resp)
}

// ✅ 비밀번호 저장
// setPasswordHandler godoc
// @Summary     비밀번호 저장
// @Description 비밀번호를 해시화하여 DB에 저장합니다.
// @Tags        Auth
// @Accept      json
// @Produce     json
// @Param       payload body map[string]string true "비밀번호 페이로드 (예: {\"password\": \"1234\"})"
// @Success     200 {object} PasswordStatusResponse
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/set-password [post]
func setPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload passwordPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil || payload.Password == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	conn := db.GetDB()
	_, err = conn.Exec("DELETE FROM passwords")
	if err != nil {
		http.Error(w, "Failed to clear old password", http.StatusInternalServerError)
		return
	}

	_, err = conn.Exec("INSERT INTO passwords (password_hash) VALUES (?)", hashed)
	if err != nil {
		http.Error(w, "Failed to store password", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(`{"status":"password saved in DB"}`))
}

// ✅ 비밀번호 확인
// verifyPasswordHandler godoc
// @Summary     비밀번호 검증
// @Description 저장된 비밀번호 해시와 비교하여 일치 여부를 확인합니다.
// @Tags        Auth
// @Accept      json
// @Produce     json
// @Param       payload body map[string]string true "비밀번호 페이로드"
// @Success     200 {object} PasswordStatusResponse
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/verify-password [post]
func verifyPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload passwordPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil || payload.Password == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	conn := db.GetDB()
	row := conn.QueryRow("SELECT password_hash FROM passwords ORDER BY id DESC LIMIT 1")

	var hashed string
	err = row.Scan(&hashed)
	if err != nil {
		http.Error(w, "No password found", http.StatusNotFound)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(hashed), []byte(payload.Password))
	if err != nil {
		http.Error(w, "Password mismatch", http.StatusUnauthorized)
		return
	}

	w.Write([]byte(`{"status":"password match"}`))
}

// ✅ 송금 가능 여부 확인
// checkSendableHandler godoc
// @Summary     송금 가능 여부
// @Description 입력한 주소/금액/개인키 기준으로 잔액이 충분한지 여부를 반환합니다.
// @Tags        Transaction
// @Accept      json
// @Produce     json
// @Param       to query string true "받는 주소"
// @Param       amount query string true "금액"
// @Param       private_key query string true "개인키"
// @Success     200 {object} SendableResponse
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/check [get]
func checkSendableHandler(w http.ResponseWriter, r *http.Request) {
	to := C.CString(r.URL.Query().Get("to"))
	amount := C.CString(r.URL.Query().Get("amount"))
	priv := C.CString(r.URL.Query().Get("private_key"))

	defer func() {
		C.free(unsafe.Pointer(to))
		C.free(unsafe.Pointer(amount))
		C.free(unsafe.Pointer(priv))
	}()

	canSend := C.check_sendable(to, amount, priv)

	resp := map[string]bool{"can_send": bool(canSend)}
	json.NewEncoder(w).Encode(resp)
}

// ✅ 실시간 가스비 정보 조회
// getGasPriceHandler godoc
// @Summary     가스비 조회
// @Description Polygon Amoy 네트워크의 현재 가스비를 반환합니다.
// @Tags        Network
// @Accept      json
// @Produce     json
// @Success     200 {object} map[string]string
// @Failure     500 {object} ErrorResponse
// @Router      /wallets/gas [get]
func getGasPriceHandler(w http.ResponseWriter, r *http.Request) {
	gasPtr := C.get_gas_price_amoy()
	defer C.free(unsafe.Pointer(gasPtr))

	gasJson := C.GoString(gasPtr)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(gasJson))
}

// ✅ 최근 트랜잭션 스캔 (Moralis)
// scanTransactionsHandler godoc
// @Summary     최근 트랜잭션 스캔
// @Description Moralis API를 통해 최근 트랜잭션 내역을 조회합니다.
// @Tags        Transaction
// @Accept      json
// @Produce     json
// @Param       address query string true "지갑 주소"
// @Success     200 {object} map[string]interface{}
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/scan [get]
func scanTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "Missing address parameter", http.StatusBadRequest)
		return
	}

	apiKey := os.Getenv("MORALIS_API_KEY")
	url := fmt.Sprintf("https://deep-index.moralis.io/api/v2.2/%s?chain=amoy", address)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	req.Header.Set("accept", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Request to Moralis failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	if resp.StatusCode != 200 {
		http.Error(w, string(body), resp.StatusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// ✅ 현재 네트워크 정보 조회
// getNetworkInfoHandler godoc
// @Summary     네트워크 정보
// @Description 현재 연결된 Polygon Amoy 네트워크의 체인 ID와 블록 번호를 반환합니다.
// @Tags        Network
// @Accept      json
// @Produce     json
// @Success     200 {object} map[string]interface{}
// @Failure     500 {object} ErrorResponse
// @Router      /wallets/network [get]
func getNetworkInfoHandler(w http.ResponseWriter, r *http.Request) {
	infoPtr := C.get_network_info()
	defer C.free(unsafe.Pointer(infoPtr))

	info := C.GoString(infoPtr)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(info))
}

// ✅ 상세 송금 가능 여부 반환
// checkSendableDetailedHandler godoc
// @Summary     송금 가능 여부 상세
// @Description 필요한 금액, 가스비, 현재 잔액 등 송금 가능 조건을 상세하게 제공합니다.
// @Tags        Transaction
// @Accept      json
// @Produce     json
// @Param       to query string true "받는 주소"
// @Param       amount query string true "금액"
// @Param       private_key query string true "개인키"
// @Success     200 {object} map[string]interface{}
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/check-detailed [get]
func checkSendableDetailedHandler(w http.ResponseWriter, r *http.Request) {
	to := C.CString(r.URL.Query().Get("to"))
	amount := C.CString(r.URL.Query().Get("amount"))
	priv := C.CString(r.URL.Query().Get("private_key"))

	defer func() {
		C.free(unsafe.Pointer(to))
		C.free(unsafe.Pointer(amount))
		C.free(unsafe.Pointer(priv))
	}()

	result := C.check_sendable_detailed(to, amount, priv)
	defer C.free(unsafe.Pointer(result))

	jsonResult := C.GoString(result)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(jsonResult))
}

// ✅ 트랜잭션 전송 및 주소 저장
// sendTransactionHandler godoc
// @Summary     트랜잭션 전송
// @Description MATIC을 송금합니다. 🔁 내부적으로 Rust FFI 사용
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Param       to query string true "받는 주소"
// @Param       amount query string true "금액 (MATIC)"
// @Param       private_key query string true "보내는 사람의 개인키"
// @Success     200 {object} TransactionResponse
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/send [get]
func sendTransactionHandler(w http.ResponseWriter, r *http.Request) {
	to := r.URL.Query().Get("to")
	amount := r.URL.Query().Get("amount")
	privateKey := r.URL.Query().Get("private_key")

	if to == "" || amount == "" || privateKey == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	toC := C.CString(to)
	amountC := C.CString(amount)
	privC := C.CString(privateKey)

	defer func() {
		C.free(unsafe.Pointer(toC))
		C.free(unsafe.Pointer(amountC))
		C.free(unsafe.Pointer(privC))
	}()

	txHash := C.GoString(C.send_transaction(toC, amountC, privC))

	saveRecentAddress(to)

	resp := map[string]string{"tx_hash": txHash}
	json.NewEncoder(w).Encode(resp)
}

// ✅ 최근 주소 리스트 반환
// getRecentAddressesHandler godoc
// @Summary     최근 송금 주소 조회
// @Description 최근에 송금한 주소 목록을 시간 순으로 반환합니다.
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Success     200 {object} []RecentAddress
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/recent [get]
func getRecentAddressesHandler(w http.ResponseWriter, r *http.Request) {
	conn := db.GetDB()
	rows, err := conn.Query("SELECT address, last_used FROM recent_addresses ORDER BY last_used DESC LIMIT 10")
	if err != nil {
		http.Error(w, "Failed to fetch recent addresses", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var addresses []map[string]string
	for rows.Next() {
		var addr string
		var lastUsed time.Time
		if err := rows.Scan(&addr, &lastUsed); err != nil {
			continue
		}
		addresses = append(addresses, map[string]string{
			"address":   addr,
			"last_used": lastUsed.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(addresses)
}

// ✅ QR코드 생성 핸들러
// generateQRCodeHandler godoc
// @Summary     QR코드 생성
// @Description 입력된 주소에 대한 QR코드를 이미지로 반환합니다.
// @Tags        Wallet
// @Accept      json
// @Produce     image/png
// @Param       address query string true "주소"
// @Success     200 {string} binary
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/qrcode [get]
func generateQRCodeHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "Missing address", http.StatusBadRequest)
		return
	}

	png, err := qrcode.Encode(address, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

// ✅ 주소 조회 핸들러
// getAddressHandler godoc
// @Summary     주소 조회
// @Description 개인키로부터 지갑 주소를 반환합니다.
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Param       private_key query string true "개인키"
// @Success     200 {object} AddressResponse
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/from-address [get]
func getAddressHandler(w http.ResponseWriter, r *http.Request) {
	privateKey := r.URL.Query().Get("private_key")
	if privateKey == "" {
		http.Error(w, "Missing private_key", http.StatusBadRequest)
		return
	}

	pkC := C.CString(privateKey)
	defer C.free(unsafe.Pointer(pkC))

	addrPtr := C.get_address_from_private_key(pkC)
	defer C.free(unsafe.Pointer(addrPtr))

	address := C.GoString(addrPtr)
	json.NewEncoder(w).Encode(map[string]string{
		"address": address,
	})
}

// ✅ private key 생성 핸들러
// generatePrivateKeyHandler godoc
// @Summary     개인키 생성
// @Description 새로운 개인키를 생성합니다.
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Success     200 {object} map[string]string
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/private-key [get]
func generatePrivateKeyHandler(w http.ResponseWriter, r *http.Request) {
	privPtr := C.generate_private_key()
	defer C.free(unsafe.Pointer(privPtr))

	privateKey := C.GoString(privPtr)
	json.NewEncoder(w).Encode(map[string]string{
		"private_key": privateKey,
	})
}

// ✅ 등록된 외부 지갑 주소 목록 조회
// getRegisteredWalletsHandler godoc
// @Summary     등록된 외부 지갑 조회
// @Description 외부 지갑 주소와 라벨 목록을 반환합니다.
// @Tags        Wallet
// @Accept      json
// @Produce     json
// @Success     200 {object} []RegisteredWallet
// @Failure     400 {object} ErrorResponse
// @Router      /wallets/registered [get]
func getRegisteredWalletsHandler(w http.ResponseWriter, r *http.Request) {
	conn := db.GetDB()
	if conn == nil {
		http.Error(w, "Database not initialized", http.StatusInternalServerError)
		return
	}

	rows, err := conn.Query(`
		SELECT address, label, registered_at 
		FROM registered_wallets 
		ORDER BY registered_at DESC
	`)
	if err != nil {
		http.Error(w, "Failed to fetch wallets", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var wallets []map[string]string
	for rows.Next() {
		var address, label string
		var registeredAt time.Time
		if err := rows.Scan(&address, &label, &registeredAt); err != nil {
			continue
		}
		wallets = append(wallets, map[string]string{
			"address":       address,
			"label":         label,
			"registered_at": registeredAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(wallets)
}

// ✅ 리팩토링: 핸들러 등록 함수 분리
func registerHandlers() {
	http.HandleFunc("/wallets/create", generateMnemonicHandler)
	http.HandleFunc("/wallets/address", generateAddressHandler)
	http.HandleFunc("/wallets/balance", getBalanceHandler)
	http.HandleFunc("/wallets/send", sendTransactionHandler)
	http.HandleFunc("/wallets/history/go", getTxHistoryHandler)
	http.HandleFunc("/wallets/history/ffi", getHistoryHandler)
	http.HandleFunc("/wallets/recover", recoverWalletHandler)
	http.HandleFunc("/wallets/verify", verifyMnemonicHandler)
	http.HandleFunc("/wallets/set-password", setPasswordHandler)
	http.HandleFunc("/wallets/verify-password", verifyPasswordHandler)
	http.HandleFunc("/wallets/check", checkSendableHandler)
	http.HandleFunc("/wallets/gas", getGasPriceHandler)
	http.HandleFunc("/wallets/scan", scanTransactionsHandler)
	http.HandleFunc("/wallets/network", getNetworkInfoHandler)
	http.HandleFunc("/wallets/check-detailed", checkSendableDetailedHandler)
	http.HandleFunc("/wallets/recent", getRecentAddressesHandler)
	http.HandleFunc("/wallets/qrcode", generateQRCodeHandler)
	http.HandleFunc("/wallets/from-address", getAddressHandler)
	http.HandleFunc("/wallets/private-key", generatePrivateKeyHandler)
	http.HandleFunc("/wallets/registered", getRegisteredWalletsHandler)
	http.Handle("/swagger/", httpSwagger.WrapHandler)
}

// ✅ 리팩토링: main 함수 간결화
func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️ .env 파일을 로드하지 못했습니다:", err)
	} else {
		log.Println("✅ .env 파일 로드 성공")
		log.Println("🔑 MORALIS_API_KEY =", os.Getenv("MORALIS_API_KEY"))
	}

	db.InitDB()
	registerHandlers()

	log.Println("🚀 Server running at http://0.0.0.0:8080")

	// ⬇️ IPv4에만 바인딩
	listener, err := net.Listen("tcp4", "0.0.0.0:8080")
	if err != nil {
		log.Fatalf("❌ 포트 리스닝 실패: %v", err)
	}

	err = http.Serve(listener, nil)
	if err != nil {
		log.Fatalf("❌ 서버 시작 실패: %v", err)
	}
}
