use bip39::{Language, Mnemonic};
use k256::ecdsa::SigningKey;
use rand::RngCore;
use sha3::{Digest, Keccak256};
use std::ffi::{CStr, CString};
use std::os::raw::c_char;
use std::str::FromStr;
use web3::types::{Address, TransactionParameters, U256};
use web3::transports::Http;
use web3::signing::{Key, SecretKey};
use tokio::runtime::Runtime;
use k256::elliptic_curve::generic_array::{GenericArray, typenum};
use std::env;
use reqwest::blocking::Client;

/// ✅ 24개 단어 니모닉을 생성
#[no_mangle]
pub extern "C" fn generate_mnemonic() -> *mut c_char {
    let mut entropy = [0u8; 32];
    rand::thread_rng().fill_bytes(&mut entropy);

    let mnemonic = match Mnemonic::from_entropy_in(Language::English, &entropy) {
        Ok(m) => m,
        Err(_) => return std::ptr::null_mut(),
    };

    CString::new(mnemonic.to_string()).unwrap().into_raw()
}

/// ✅ 새로운 지갑 주소 생성
#[no_mangle]
pub extern "C" fn generate_address() -> *mut c_char {
    let mut entropy = [0u8; 32];
    rand::thread_rng().fill_bytes(&mut entropy);

    let signing_key = SigningKey::from_bytes((&entropy).into()).unwrap();
    let verifying_key = signing_key.verifying_key();
    let public_key = verifying_key.to_encoded_point(false);
    let public_key_bytes = public_key.as_bytes();

    let hash = Keccak256::digest(&public_key_bytes[1..]);
    let address = format!("0x{}", hex::encode(&hash[12..]));

    CString::new(address).unwrap().into_raw()
}

/// ✅ 특정 주소의 잔액을 조회
#[no_mangle]
pub extern "C" fn get_balance_by_address(address: *const c_char) -> *mut c_char {
    if address.is_null() {
        return CString::new("Invalid address").unwrap().into_raw();
    }

    let addr_str = unsafe { CStr::from_ptr(address).to_string_lossy().into_owned() };
    let parsed_address = match Address::from_str(&addr_str) {
        Ok(a) => a,
        Err(_) => return CString::new("Invalid address format").unwrap().into_raw(),
    };

    let rt = Runtime::new().unwrap();
    let result = rt.block_on(async {
        let http = Http::new("https://rpc-amoy.polygon.technology").unwrap();
        let web3 = web3::Web3::new(http);

        match web3.eth().balance(parsed_address, None).await {
            Ok(balance) => {
                let matic_float = balance.as_u128() as f64 / 1e18;
                // JSON 형태로 반환
                format!(
                    "{{\"balance\": \"{:.6}\", \"symbol\": \"MATIC\", \"network\": \"polygon-amoy\"}}",
                    matic_float
                )
            },
            Err(_) => {
                "{\"balance\": \"0.000000\", \"symbol\": \"MATIC\", \"network\": \"polygon-amoy\"}".to_string()
            },
        }
    });

    CString::new(result).unwrap().into_raw()
}

/// ✅ 트랜잭션 전송 후 트랜잭션 해시 반환
#[no_mangle]
pub extern "C" fn send_transaction(to: *const c_char, amount: *const c_char, private_key: *const c_char) -> *mut c_char {
    let to_str = unsafe { CStr::from_ptr(to).to_str().unwrap_or_default() };
    let amount_str = unsafe { CStr::from_ptr(amount).to_str().unwrap_or("0") };
    let private_key_str = unsafe { CStr::from_ptr(private_key).to_str().unwrap_or_default() };

    let runtime = Runtime::new().unwrap();

    let result = runtime.block_on(async {
        let transport = Http::new("https://rpc-amoy.polygon.technology").unwrap();
        let web3 = web3::Web3::new(transport);

        let pk = match SecretKey::from_str(private_key_str) {
            Ok(k) => k,
            Err(_) => return Err("Invalid private key"),
        };

        let from = (&pk).address();

        let to_addr = match Address::from_str(to_str) {
            Ok(addr) => addr,
            Err(_) => return Err("Invalid recipient address"),
        };

        let amount_wei = match amount_str.parse::<f64>() {
            Ok(amount_f64) => {
                let wei_f64 = amount_f64 * 1e18;
                U256::from(wei_f64 as u128)
            },
            Err(_) => return Err("Invalid amount format"),
        };

        let tx = TransactionParameters {
            to: Some(to_addr),
            value: amount_wei,
            gas: 21000.into(),
            ..Default::default()
        };

        let signed = web3.accounts().sign_transaction(tx, &pk).await.map_err(|_| "Signing failed")?;
        let tx_hash = web3.eth().send_raw_transaction(signed.raw_transaction).await.map_err(|_| "Send failed")?;

        Ok(format!("{:?}", tx_hash))
    });

    match result {
        Ok(tx_hash) => CString::new(tx_hash).unwrap().into_raw(),
        Err(e) => CString::new(e).unwrap().into_raw(),
    }
}

/// ✅ FFI 메모리 해제 함수
#[no_mangle]
pub extern "C" fn free_string(s: *mut c_char) {
    if s.is_null() { return; }
    unsafe {
        drop(CString::from_raw(s));
    }
}

/// ✅ Moralis를 통한 트랜잭션 내역 조회
#[no_mangle]
pub extern "C" fn get_transaction_history(address: *const c_char) -> *mut c_char {
    let address = unsafe {
        if address.is_null() {
            return CString::new("invalid address").unwrap().into_raw();
        }
        CStr::from_ptr(address).to_string_lossy().into_owned()
    };

    let api_key = env::var("MORALIS_API_KEY").unwrap_or_else(|_| "".to_string());
    let url = format!(
        "https://mainnet-aptos-api.moralis.io/v2/{}?chain=amoy",
        address
    );

    let client = Client::new();
    let response = client
        .get(&url)
        .header("accept", "application/json")
        .header("X-API-Key", api_key)
        .send();

    match response {
        Ok(resp) => match resp.text() {
            Ok(text) => CString::new(text).unwrap().into_raw(),
            Err(_) => CString::new("Failed to parse response").unwrap().into_raw(),
        },
        Err(_) => CString::new("Request to Moralis failed").unwrap().into_raw(),
    }
}

/// ✅ 니모닉으로 지갑 복구 (주소와 개인키 반환)
#[no_mangle]
pub extern "C" fn recover_wallet_from_mnemonic(mnemonic: *const c_char) -> *mut c_char {
    let mnemonic_str = unsafe {
        if mnemonic.is_null() {
            return CString::new("Invalid mnemonic").unwrap().into_raw();
        }
        CStr::from_ptr(mnemonic).to_string_lossy().into_owned()
    };

    let mnemonic = match Mnemonic::from_str(&mnemonic_str) {
        Ok(m) => m,
        Err(_) => return CString::new("Invalid mnemonic").unwrap().into_raw(),
    };

    let seed = mnemonic.to_seed("");
    
    let seed_bytes: &GenericArray<u8, typenum::U32> = &GenericArray::clone_from_slice(&seed[0..32]);

    let signing_key = match SigningKey::from_bytes(seed_bytes) {
        Ok(key) => key,
        Err(_) => return CString::new("Failed to create signing key").unwrap().into_raw(),
    };

    let private_key_bytes = signing_key.to_bytes();
    let private_key_hex = hex::encode(private_key_bytes);
    let secret_key = match SecretKey::from_str(&private_key_hex) {
        Ok(sk) => sk,
        Err(_) => return CString::new("Failed to parse private key").unwrap().into_raw(),
    };

    // 주소 생성
    let public_key = signing_key.verifying_key().to_encoded_point(false);
    let public_key_bytes = public_key.as_bytes();
    let hash = Keccak256::digest(&public_key_bytes[1..]);
    let address = format!("0x{}", hex::encode(&hash[12..]));

    let result = format!(
        "{{\"address\": \"{}\", \"private_key\": \"0x{}\"}}",
        address, private_key_hex
    );

    CString::new(result).unwrap().into_raw()
}

/// ✅ 니모닉 구문 유효성 검증
#[no_mangle]
pub extern "C" fn verify_mnemonic(mnemonic: *const c_char) -> bool {
    if mnemonic.is_null() {
        return false;
    }
    let mnemonic_str = unsafe { CStr::from_ptr(mnemonic).to_string_lossy().into_owned() };

    Mnemonic::from_str(&mnemonic_str).is_ok()
}

/// ✅ 송금 가능 여부 확인 (true/false)
#[no_mangle]
pub extern "C" fn check_sendable(to: *const c_char, amount: *const c_char, private_key: *const c_char) -> bool {
    let to = unsafe { CStr::from_ptr(to).to_string_lossy().to_string() };
    let amount = unsafe { CStr::from_ptr(amount).to_string_lossy().to_string() };
    let private_key = unsafe { CStr::from_ptr(private_key).to_string_lossy().to_string() };

    let rt = Runtime::new().unwrap();
    let result: Result<bool, ()> = rt.block_on(async {
        let web3 = web3::Web3::new(Http::new("https://rpc-amoy.polygon.technology").unwrap());
        let key = SecretKey::from_str(&private_key).map_err(|_| ())?;
        let from = (&key).address();
        let balance = web3.eth().balance(from, None).await.map_err(|_| ())?;

        let amount_wei = amount.parse::<f64>().ok()
            .map(|amt| (amt * 1e18) as u128)
            .map(U256::from)
            .ok_or(())?;

        let gas_price = web3.eth().gas_price().await.map_err(|_| ())?;
        let gas_limit = U256::from(21000);
        let gas_fee = gas_price * gas_limit;

        if balance >= (amount_wei + gas_fee) {
            Ok(true)
        } else {
            Ok(false)
        }
    });

    matches!(result, Ok(true))
}

// /// ✅ 이더스캔 API를 통한 가스비 정보 조회
// #[no_mangle]
// pub extern "C" fn get_gas_price() -> *mut c_char {
//     use reqwest::blocking::Client;
//     use serde::Deserialize;

//     #[derive(Deserialize)]
//     struct GasOracleResponse {
//         status: String,
//         message: String,
//         result: GasOracleResult,
//     }

//     #[derive(Deserialize)]
//     struct GasOracleResult {
//         SafeGasPrice: String,
//         ProposeGasPrice: String,
//         FastGasPrice: String,
//         suggestBaseFee: String,
//         gasUsedRatio: String,
//     }

//     let api_key = std::env::var("ETHERSCAN_API_KEY").unwrap_or_else(|_| "".to_string());
//     let url = format!("https://api.etherscan.io/api?module=gastracker&action=gasoracle&apikey={}", api_key);

//     let client = Client::new();
//     let response = client.get(&url).send();

//     match response {
//         Ok(resp) => {
//             match resp.json::<GasOracleResponse>() {
//                 Ok(parsed) => {
//                     let gas_info = format!(
//                         "{{\"SafeGasPrice\": \"{}\", \"ProposeGasPrice\": \"{}\", \"FastGasPrice\": \"{}\", \"suggestBaseFee\": \"{}\", \"gasUsedRatio\": \"{}\"}}",
//                         parsed.result.SafeGasPrice,
//                         parsed.result.ProposeGasPrice,
//                         parsed.result.FastGasPrice,
//                         parsed.result.suggestBaseFee,
//                         parsed.result.gasUsedRatio
//                     );
//                     CString::new(gas_info).unwrap().into_raw()
//                 },
//                 Err(_) => CString::new("Failed to parse gas price response").unwrap().into_raw(),
//             }
//         },
//         Err(_) => CString::new("Gas price request failed").unwrap().into_raw(),
//     }
// }

/// ✅ Polygon Amoy 네트워크의 실시간 가스비를 조회 (RPC 기반)
#[no_mangle]
pub extern "C" fn get_gas_price_amoy() -> *mut c_char {
    use web3::transports::Http;
    use web3::types::U256;
    use std::ffi::CString;
    use tokio::runtime::Runtime;

    let rpc_url = "https://rpc-amoy.polygon.technology";

    let rt = Runtime::new().unwrap();
    let result = rt.block_on(async {
        let transport = Http::new(rpc_url).unwrap();
        let web3 = web3::Web3::new(transport);

        match web3.eth().gas_price().await {
            Ok(gas_price) => {
                let gwei = gas_price / U256::exp10(9); // Wei → Gwei 변환
                format!("{{\"gas_price_wei\": \"{}\", \"gas_price_gwei\": \"{}\", \"network\": \"polygon-amoy\"}}",
                        gas_price, gwei)
            }
            Err(_) => {
                "{\"error\": \"Failed to fetch gas price from Polygon Amoy\"}".to_string()
            }
        }
    });

    CString::new(result).unwrap().into_raw()
}


/// ✅ 현재 네트워크 정보 조회
#[no_mangle]
pub extern "C" fn get_network_info() -> *mut c_char {
    let rt = Runtime::new().unwrap();
    let result = rt.block_on(async {
        let http = Http::new("https://rpc-amoy.polygon.technology").unwrap();
        let web3 = web3::Web3::new(http);

        let net_version = web3.net().version().await.unwrap_or_else(|_| "unknown".to_string());
        let block_number = web3.eth().block_number().await.map(|n| n.as_u64()).unwrap_or(0);

        let json = format!(
            "{{\"network\": \"polygon-amoy\", \"chain_id\": {}, \"block_number\": {}}}",
            net_version, block_number
        );

        json
    });

    CString::new(result).unwrap().into_raw()
}

/// ✅ 송금 가능 여부 상세 확인 (잔액/가스 포함 JSON 반환)
#[no_mangle]
pub extern "C" fn check_sendable_detailed(to: *const c_char, amount: *const c_char, private_key: *const c_char) -> *mut c_char {
    use std::str::FromStr;

    let to = unsafe { CStr::from_ptr(to).to_string_lossy().to_string() };
    let amount = unsafe { CStr::from_ptr(amount).to_string_lossy().to_string() };
    let private_key = unsafe { CStr::from_ptr(private_key).to_string_lossy().to_string() };

    let rt = Runtime::new().unwrap();
    let result: Result<String, ()> = rt.block_on(async {
        let web3 = web3::Web3::new(Http::new("https://rpc-amoy.polygon.technology").unwrap());
        let key = SecretKey::from_str(&private_key).map_err(|_| ())?;
        let from = (&key).address();
        let balance = web3.eth().balance(from, None).await.map_err(|_| ())?;

        let amount_eth = amount.parse::<f64>().map_err(|_| ())?;
        let amount_wei = U256::from((amount_eth * 1e18) as u128);

        let gas_price = web3.eth().gas_price().await.map_err(|_| ())?;
        let gas_limit = U256::from(21000);
        let gas_fee = gas_price * gas_limit;

        let required = amount_wei + gas_fee;

        let can_send = balance >= required;

        let response = if can_send {
            format!(r#"{{"can_send": true, "balance_eth": {:.6}, "required_eth": {:.6}}}"#,
                balance.as_u128() as f64 / 1e18,
                required.as_u128() as f64 / 1e18
            )
        } else {
            format!(r#"{{"can_send": false, "balance_eth": {:.6}, "required_eth": {:.6}, "reason": "잔액 부족"}}"#,
                balance.as_u128() as f64 / 1e18,
                required.as_u128() as f64 / 1e18
            )
        };

        Ok(response)
    });

    match result {
        Ok(json) => CString::new(json).unwrap().into_raw(),
        Err(_) => CString::new(r#"{"can_send": false, "error": "계산 실패"}"#).unwrap().into_raw(),
    }
}

/// ✅ 내 지갑 주소 반환
#[no_mangle]
pub extern "C" fn get_address_from_private_key(pk_ptr: *const c_char) -> *mut c_char {
    let pk_str = unsafe { CStr::from_ptr(pk_ptr).to_string_lossy().to_string() };
    let sk = match SecretKey::from_str(&pk_str) {
        Ok(s) => s,
        Err(_) => return CString::new("Invalid private key").unwrap().into_raw(),
    };

    let address = format!("{:#x}", (&sk).address());
    CString::new(address).unwrap().into_raw()
}

/// ✅ private key 생성
#[no_mangle]
pub extern "C" fn generate_private_key() -> *mut c_char {
    let mut entropy = [0u8; 32];
    rand::thread_rng().fill_bytes(&mut entropy);

    let signing_key = SigningKey::from_bytes((&entropy).into()).unwrap();
    let private_key_hex = hex::encode(signing_key.to_bytes());

    CString::new(private_key_hex).unwrap().into_raw()
}
