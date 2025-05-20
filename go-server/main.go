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
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"

	//"os"
	"unsafe"
)

type passwordPayload struct {
	Password string `json:"password"`
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
func generateMnemonicHandler(w http.ResponseWriter, r *http.Request) {
	mnemonicPtr := C.generate_mnemonic()
	mnemonic := C.GoString(mnemonicPtr)
	C.free(unsafe.Pointer(mnemonicPtr))

	resp := map[string]string{"mnemonic": mnemonic}
	json.NewEncoder(w).Encode(resp)
}

// ✅ 새로운 지갑 주소 생성
func generateAddressHandler(w http.ResponseWriter, r *http.Request) {
	addrPtr := C.generate_address()
	address := C.GoString(addrPtr)
	C.free(unsafe.Pointer(addrPtr))

	resp := map[string]string{"address": address}
	json.NewEncoder(w).Encode(resp)
}

// ✅ 현재 지갑 잔액 조회
func getBalanceHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "Missing address parameter", http.StatusBadRequest)
		return
	}

	addrC := C.CString(address)
	defer C.free(unsafe.Pointer(addrC))

	// 	resultPtr := C.get_balance_by_address(addrC)
	// 	defer C.free(unsafe.Pointer(resultPtr))

	// 	balance := C.GoString(resultPtr)
	// 	resp := map[string]string{"balance": balance + " ETH"}
	// 	json.NewEncoder(w).Encode(resp)
	resultPtr := C.get_balance_by_address(addrC)
	defer C.free(unsafe.Pointer(resultPtr))

	balanceJson := C.GoString(resultPtr)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(balanceJson))
}

// }

// ✅ 거래 내역(Moralis API) 가져오기
func getTxHistoryHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "Missing address parameter", http.StatusBadRequest)
		return
	}

	apiKey := os.Getenv("MORALIS_API_KEY")
	if apiKey == "" {
		log.Println("❌ MORALIS_API_KEY is not set")
		http.Error(w, `{"error": "MORALIS_API_KEY is not set on the server"}`, http.StatusInternalServerError)
		return
	}

	url := fmt.Sprintf("https://deep-index.moralis.io/api/v2/%s?chain=amoy", address)

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
func getGasPriceHandler(w http.ResponseWriter, r *http.Request) {
	gasPtr := C.get_gas_price_amoy()
	defer C.free(unsafe.Pointer(gasPtr))

	gasJson := C.GoString(gasPtr)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(gasJson))
}

// ✅ 최근 트랜잭션 스캔 (Moralis)
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
func getNetworkInfoHandler(w http.ResponseWriter, r *http.Request) {
	infoPtr := C.get_network_info()
	defer C.free(unsafe.Pointer(infoPtr))

	info := C.GoString(infoPtr)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(info))
}

// ✅ 상세 송금 가능 여부 반환
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
func generatePrivateKeyHandler(w http.ResponseWriter, r *http.Request) {
	privPtr := C.generate_private_key()
	defer C.free(unsafe.Pointer(privPtr))

	privateKey := C.GoString(privPtr)
	json.NewEncoder(w).Encode(map[string]string{
		"private_key": privateKey,
	})
}

// ✅ 등록된 외부 지갑 주소 목록 조회
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

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️ .env 파일을 로드하지 못했습니다:", err)
	} else {
		log.Println("✅ .env 파일 로드 성공")
		log.Println("🔑 MORALIS_API_KEY =", os.Getenv("MORALIS_API_KEY"))
	}
	db.InitDB()
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

	log.Println("🚀 Server running at http://localhost:8080")
	err = http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		log.Fatalf("❌ 서버 시작 실패: %v", err)
	}
	//log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
}
