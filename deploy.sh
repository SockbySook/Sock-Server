#!/bin/bash

# Rust 빌드
cd rust-wallet-lib
cargo build --release
cd ..

# Go 빌드
cd go-server
go build -o myapp ./...
cd ..

# systemd 서비스는 아직 없으니 주석 처리
# sudo systemctl restart myapp
