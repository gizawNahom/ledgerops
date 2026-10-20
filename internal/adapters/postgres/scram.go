package postgres

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"

	"golang.org/x/text/secure/precis"

	"ledgerops/internal/support"
)

const scramKeyLength = sha256.Size

// BuildSCRAMVerifier builds a SCRAM-SHA-256 verifier with SASLprep
// normalisation (DDD-47); salt and iterations are injected so the function is
// pure. The plaintext is never part of the result.
func BuildSCRAMVerifier(password support.Secret, salt []byte, iterations int) string {
	salted := pbkdf2SHA256([]byte(saslPrep(password.Reveal())), salt, iterations)
	clientKey := hmacSHA256(salted, []byte("Client Key"))
	storedKey := sha256.Sum256(clientKey)
	serverKey := hmacSHA256(salted, []byte("Server Key"))
	encode := base64.StdEncoding.EncodeToString
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s",
		iterations, encode(salt), encode(storedKey[:]), encode(serverKey))
}

// saslPrep normalises as the server does: a password SASLprep rejects is used
// as given.
func saslPrep(password string) string {
	prepared, err := precis.OpaqueString.String(password)
	if err != nil {
		return password
	}
	return prepared
}

func hmacSHA256(key, message []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

// pbkdf2SHA256 is PBKDF2-HMAC-SHA256 producing one 32-byte block (RFC 8018).
func pbkdf2SHA256(password, salt []byte, iterations int) []byte {
	var blockIndex [4]byte
	binary.BigEndian.PutUint32(blockIndex[:], 1)
	u := hmacSHA256(password, append(append([]byte{}, salt...), blockIndex[:]...))
	derived := append([]byte{}, u...)
	for i := 1; i < iterations; i++ {
		u = hmacSHA256(password, u)
		for j := range derived[:scramKeyLength] {
			derived[j] ^= u[j]
		}
	}
	return derived
}
