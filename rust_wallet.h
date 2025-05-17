#ifndef RUST_WALLET_H
#define RUST_WALLET_H

#include <stdarg.h>
#include <stdint.h>
#include <stdlib.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

char *generate_mnemonic(void);
char *generate_address(void);
char *get_balance_by_address(const char *address);
char *send_transaction(const char *to, const char *amount, const char *private_key);
void free_string(char *s);
char *get_transaction_history(const char *address);
char *recover_wallet_from_mnemonic(const char *mnemonic);
bool verify_mnemonic(const char *mnemonic);
bool check_sendable(const char *to, const char *amount, const char *private_key);
char *get_gas_price(void);
char *get_network_info(void);
char *check_sendable_detailed(const char *to, const char *amount, const char *private_key);
char *get_address_from_private_key(const char *pk_ptr);
char *generate_private_key(void);

#ifdef __cplusplus
}
#endif

#endif // RUST_WALLET_H
