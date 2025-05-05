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

// InitDB initializes the SQLite database and creates the recent_addresses table
func InitDB() {
	once.Do(func() {
		var err error
		db, err = sql.Open("sqlite3", "./recent_addresses.db")
		if err != nil {
			log.Fatalf("❌ DB 연결 실패: %v", err)
		}

		createTable := `
		CREATE TABLE IF NOT EXISTS recent_addresses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			address TEXT NOT NULL UNIQUE,
			last_used TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`

		_, err = db.Exec(createTable)
		if err != nil {
			log.Fatalf("❌ 테이블 생성 실패: %v", err)
		}

		log.Println("✅ recent_addresses 테이블 초기화 완료")
	})
}

// GetDB returns the initialized DB instance
func GetDB() *sql.DB {
	if db == nil {
		log.Fatal("❌ DB가 초기화되지 않았습니다. InitDB()를 먼저 호출하세요.")
	}
	return db
}
