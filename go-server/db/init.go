package db

import (
	"database/sql"
	"log"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var (
	db   *sql.DB
	once sync.Once
)

func InitDB() {
	once.Do(func() {
		var err error
		db, err = sql.Open("sqlite3", "./recent_addresses.db")
		if err != nil {
			log.Fatalf("❌ DB 연결 실패: %v", err)
		}

		// 최근 송금 주소 테이블 생성
		createRecentAddressesTable := `
		CREATE TABLE IF NOT EXISTS recent_addresses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			address TEXT NOT NULL UNIQUE,
			last_used TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`
		_, err = db.Exec(createRecentAddressesTable)
		if err != nil {
			log.Fatalf("❌ recent_addresses 테이블 생성 실패: %v", err)
		}
		log.Println("✅ recent_addresses 테이블 초기화 완료")

		// 비밀번호 해시 저장 테이블 생성
		createPasswordTable := `
		CREATE TABLE IF NOT EXISTS passwords (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			password_hash TEXT NOT NULL
		);`
		_, err = db.Exec(createPasswordTable)
		if err != nil {
			log.Fatalf("❌ passwords 테이블 생성 실패: %v", err)
		}
		log.Println("✅ passwords 테이블 초기화 완료")
	})
}

func GetDB() *sql.DB {
	if db == nil {
		log.Fatal("❌ DB가 초기화되지 않았습니다. InitDB()를 먼저 호출하세요.")
	}
	return db
}
