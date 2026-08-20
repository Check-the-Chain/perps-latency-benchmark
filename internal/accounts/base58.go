package accounts

import "math/big"

// base58Alphabet is the Bitcoin/Solana alphabet. Bullet addresses and ed25519
// delegate public keys are base58-encoded: the REST API rejects the hex form
// of the same bytes with "invalid address".
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func encodeBase58(input []byte) string {
	if len(input) == 0 {
		return ""
	}
	value := new(big.Int).SetBytes(input)
	radix := big.NewInt(58)
	remainder := new(big.Int)
	encoded := make([]byte, 0, len(input)*137/100+1)
	for value.Sign() > 0 {
		value.DivMod(value, radix, remainder)
		encoded = append(encoded, base58Alphabet[remainder.Int64()])
	}
	// Leading zero bytes are not captured by the big.Int conversion and each
	// encodes as the alphabet's zero digit.
	for _, b := range input {
		if b != 0 {
			break
		}
		encoded = append(encoded, base58Alphabet[0])
	}
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	return string(encoded)
}
