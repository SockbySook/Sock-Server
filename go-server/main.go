package main

/*
#cgo LDFLAGS: -L. -lrust_wallet
#include <stdlib.h>
#include <stdbool.h>

char* generate_mnemonic();
char* generate_address();
char* get_balance();
char* send_transaction(const char* to, const char* amount, const char* private_key);
char* get_transaction_history(const char* address);
char* recover_wallet_from_mnemonic(const char* mnemonic);
bool verify_mnemonic(const char* mnemonic);
bool check_sendable(const char* to, const char* amount, const char* private_key);
char* get_gas_price();
char* get_network_info();
char* check_sendable_detailed(const char* to, const char* amount, const char* private_key);
char* check_sendable_detailed(const char* to, const char* amount, const char* private_key);

*/
import "C"

import (
	"encoding/json"
	"fmt"
	"go-server/db"
	"io/ioutil"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	//"os"
	"unsafe"
)

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

func generateMnemonicHandler(w http.ResponseWriter, r *http.Request) {
	mnemonicPtr := C.generate_mnemonic()
	mnemonic := C.GoString(mnemonicPtr)
	C.free(unsafe.Pointer(mnemonicPtr))

	resp := map[string]string{"mnemonic": mnemonic}
	json.NewEncoder(w).Encode(resp)
}

func generateAddressHandler(w http.ResponseWriter, r *http.Request) {
	addrPtr := C.generate_address()
	address := C.GoString(addrPtr)
	C.free(unsafe.Pointer(addrPtr))

	resp := map[string]string{"address": address}
	json.NewEncoder(w).Encode(resp)
}

func getBalanceHandler(w http.ResponseWriter, r *http.Request) {
	balancePtr := C.get_balance()
	balance := C.GoString(balancePtr)
	C.free(unsafe.Pointer(balancePtr))

	resp := map[string]string{"balance": balance + " ETH"}
	json.NewEncoder(w).Encode(resp)
}

func getTxHistoryHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "Missing address parameter", http.StatusBadRequest)
		return
	}

	apiKey := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJub25jZSI6ImZiOTgwYzRlLWZjNjUtNDVhZi1hMTcyLTg0NTU4NGM5ZGJjMCIsIm9yZ0lkIjoiNDQ1MDgwIiwidXNlcklkIjoiNDU3OTMyIiwidHlwZUlkIjoiNzg5M2VjOTQtZDA5Yy00YWI3LTgwM2EtYzQ1YzMyMzdlNDExIiwidHlwZSI6IlBST0pFQ1QiLCJpYXQiOjE3NDYyODEwMzgsImV4cCI6NDkwMjA0MTAzOH0.dSuxIQmJ-4yWQqba9-nYUz5RNOtVflmQLJR_WulLQ-8"
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

	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		fmt.Println("❌ Moralis API error:", string(body))
		http.Error(w, "Failed to fetch transactions", resp.StatusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

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

func recoverWalletHandler(w http.ResponseWriter, r *http.Request) {
	// URL에서 "mnemonic" 파라미터를 추출합니다.
	mnemonic := r.URL.Query().Get("mnemonic")
	if mnemonic == "" {
		http.Error(w, "Missing mnemonic parameter", http.StatusBadRequest)
		return
	}

	// C 함수 호출을 위한 CString으로 변환합니다.
	mnemonicC := C.CString(mnemonic)
	defer C.free(unsafe.Pointer(mnemonicC))

	// Rust 함수 호출
	resultPtr := C.recover_wallet_from_mnemonic(mnemonicC)
	defer C.free(unsafe.Pointer(resultPtr))

	// Rust에서 반환된 결과를 문자열로 변환
	result := C.GoString(resultPtr)

	// JSON 응답 작성
	var response map[string]string
	if result == "Invalid mnemonic" {
		response = map[string]string{"error": "Invalid mnemonic"}
	} else {
		// 복구된 지갑 주소와 개인키를 반환
		response = map[string]string{"wallet": result}
	}

	// JSON 응답을 반환
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

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

func setPasswordHandler(w http.ResponseWriter, r *http.Request) {
	password := r.URL.Query().Get("password")
	if password == "" {
		http.Error(w, "Password is required", http.StatusBadRequest)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	err = ioutil.WriteFile("password.hash", hashed, 0644)
	if err != nil {
		http.Error(w, "Failed to save password", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(`{"status":"password saved"}`))
}

func verifyPasswordHandler(w http.ResponseWriter, r *http.Request) {
	password := r.URL.Query().Get("password")
	if password == "" {
		http.Error(w, "Password is required", http.StatusBadRequest)
		return
	}

	hashed, err := ioutil.ReadFile("password.hash")
	if err != nil {
		http.Error(w, "No saved password found", http.StatusNotFound)
		return
	}

	err = bcrypt.CompareHashAndPassword(hashed, []byte(password))
	if err != nil {
		http.Error(w, "Password mismatch", http.StatusUnauthorized)
		return
	}

	w.Write([]byte(`{"status":"password match"}`))
}

// 비밀번호 해시 저장용 변수 (실제론 DB에 저장)
var passwordHash []byte

func storePasswordHandler(w http.ResponseWriter, r *http.Request) {
	pw := r.URL.Query().Get("password")
	hash, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	passwordHash = hash
	w.Write([]byte("Password saved"))
}

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

func getGasPriceHandler(w http.ResponseWriter, r *http.Request) {
	gasPtr := C.get_gas_price()
	defer C.free(unsafe.Pointer(gasPtr))

	gasJson := C.GoString(gasPtr)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(gasJson))
}

func scanTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "Missing address parameter", http.StatusBadRequest)
		return
	}

	apiKey := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJub25jZSI6ImZiOTgwYzRlLWZjNjUtNDVhZi1hMTcyLTg0NTU4NGM5ZGJjMCIsIm9yZ0lkIjoiNDQ1MDgwIiwidXNlcklkIjoiNDU3OTMyIiwidHlwZUlkIjoiNzg5M2VjOTQtZDA5Yy00YWI3LTgwM2EtYzQ1YzMyMzdlNDExIiwidHlwZSI6IlBST0pFQ1QiLCJpYXQiOjE3NDYyODEwMzgsImV4cCI6NDkwMjA0MTAzOH0.dSuxIQmJ-4yWQqba9-nYUz5RNOtVflmQLJR_WulLQ-8"
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

	body, err := ioutil.ReadAll(resp.Body)
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

func getNetworkInfoHandler(w http.ResponseWriter, r *http.Request) {
	infoPtr := C.get_network_info()
	defer C.free(unsafe.Pointer(infoPtr))

	info := C.GoString(infoPtr)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(info))
}

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

func main() {
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
	http.HandleFunc("/auth/password", storePasswordHandler)
	http.HandleFunc("/wallets/check", checkSendableHandler)
	http.HandleFunc("/wallets/gas", getGasPriceHandler)
	http.HandleFunc("/wallets/scan", scanTransactionsHandler)
	http.HandleFunc("/wallets/network", getNetworkInfoHandler)
	http.HandleFunc("/wallets/check-detailed", checkSendableDetailedHandler)
	http.HandleFunc("/wallets/recent", getRecentAddressesHandler)

	fmt.Println("🚀 Server running at http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
