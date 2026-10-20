package postgres

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/secure/precis"
	"golang.org/x/text/unicode/norm"

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

// saslPrep normalises as PostgreSQL's pg_saslprep does (RFC 4013): ASCII is
// used as given; anything else has its non-ASCII spaces mapped to a space and
// its mapped-to-nothing code points dropped, then is folded to NFKC (which is
// what turns fullwidth forms into their ASCII equivalents). A password the
// server's prohibited-character check would reject is used as given, as the
// server does.
func saslPrep(password string) string {
	if isASCII(password) {
		return password
	}
	normalized := norm.NFKC.String(strings.Map(mapForSASLprep, password))
	if _, err := precis.OpaqueString.String(normalized); err != nil {
		return password
	}
	return normalized
}

func isASCII(text string) bool {
	for index := 0; index < len(text); index++ {
		if text[index] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// mapForSASLprep is the RFC 3454 mapping step: table C.1.2 spaces to U+0020,
// table B.1 code points to nothing.
func mapForSASLprep(r rune) rune {
	switch {
	case unicode.Is(unicode.Zs, r):
		return ' '
	case r == 0x00AD, r == 0x034F, r == 0x1806, r >= 0x180B && r <= 0x180D,
		r >= 0x200B && r <= 0x200D, r == 0x2060, r >= 0xFE00 && r <= 0xFE0F, r == 0xFEFF:
		return -1
	}
	return r
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
