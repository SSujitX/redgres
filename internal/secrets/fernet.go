package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"
)

// ErrInvalidToken is the single public Decrypt failure class.
// It never includes the key, token, or plaintext.
var ErrInvalidToken = errors.New("invalid token")

const (
	fernetVersion    = 0x80
	fernetSigningLen = 16
	fernetEncKeyLen  = 16
	fernetKeyLen     = fernetSigningLen + fernetEncKeyLen
	fernetVersionLen = 1
	fernetTSLen      = 8
	fernetIVLen      = aes.BlockSize
	fernetHMACLen    = sha256.Size
	fernetMinLen     = fernetVersionLen + fernetTSLen + fernetIVLen + aes.BlockSize + fernetHMACLen
)

// Encrypt produces a Fernet token (version 0x80). No TTL is encoded as expiry;
// the timestamp is the current Unix seconds. key is a URL-safe Base64 Fernet key.
func Encrypt(key string, plaintext []byte) (string, error) {
	keyBytes, err := decodeURLBase64(key)
	if err != nil || len(keyBytes) != fernetKeyLen {
		return "", ErrInvalidToken
	}
	iv := make([]byte, fernetIVLen)
	if _, err := rand.Read(iv); err != nil {
		return "", ErrInvalidToken
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	block, err := aes.NewCipher(keyBytes[fernetSigningLen:])
	if err != nil {
		return "", ErrInvalidToken
	}
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	token := make([]byte, fernetVersionLen+fernetTSLen+fernetIVLen+len(ciphertext)+fernetHMACLen)
	token[0] = fernetVersion
	binary.BigEndian.PutUint64(token[fernetVersionLen:fernetVersionLen+fernetTSLen], uint64(time.Now().Unix()))
	copy(token[fernetVersionLen+fernetTSLen:fernetVersionLen+fernetTSLen+fernetIVLen], iv)
	copy(token[fernetVersionLen+fernetTSLen+fernetIVLen:len(token)-fernetHMACLen], ciphertext)

	mac := hmac.New(sha256.New, keyBytes[:fernetSigningLen])
	mac.Write(token[:len(token)-fernetHMACLen])
	copy(token[len(token)-fernetHMACLen:], mac.Sum(nil))
	return base64.URLEncoding.EncodeToString(token), nil
}

// Decrypt recovers Fernet plaintext. No TTL is applied; old timestamps succeed.
// key is a URL-safe Base64 Fernet key (32 raw bytes).
func Decrypt(key, token string) ([]byte, error) {
	keyBytes, err := decodeURLBase64(key)
	if err != nil || len(keyBytes) != fernetKeyLen {
		return nil, ErrInvalidToken
	}
	data, err := decodeURLBase64(token)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if len(data) < fernetMinLen || data[0] != fernetVersion {
		return nil, ErrInvalidToken
	}

	signingKey := keyBytes[:fernetSigningLen]
	encryptionKey := keyBytes[fernetSigningLen:]
	signed := data[:len(data)-fernetHMACLen]
	givenMAC := data[len(data)-fernetHMACLen:]
	mac := hmac.New(sha256.New, signingKey)
	mac.Write(signed)
	if subtle.ConstantTimeCompare(mac.Sum(nil), givenMAC) != 1 {
		return nil, ErrInvalidToken
	}

	iv := data[fernetVersionLen+fernetTSLen : fernetVersionLen+fernetTSLen+fernetIVLen]
	ciphertext := data[fernetVersionLen+fernetTSLen+fernetIVLen : len(data)-fernetHMACLen]
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, ErrInvalidToken
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, ErrInvalidToken
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	unpadded, ok := pkcs7Unpad(plaintext, aes.BlockSize)
	if !ok {
		return nil, ErrInvalidToken
	}
	return unpadded, nil
}

func decodeURLBase64(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 1:
		return nil, ErrInvalidToken
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func pkcs7Pad(buf []byte, blockSize int) []byte {
	pad := blockSize - (len(buf) % blockSize)
	out := make([]byte, len(buf)+pad)
	copy(out, buf)
	for i := len(buf); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(buf []byte, blockSize int) ([]byte, bool) {
	if len(buf) == 0 || len(buf)%blockSize != 0 {
		return nil, false
	}
	pad := int(buf[len(buf)-1])
	if pad < 1 || pad > blockSize || pad > len(buf) {
		return nil, false
	}
	for _, b := range buf[len(buf)-pad:] {
		if int(b) != pad {
			return nil, false
		}
	}
	return buf[:len(buf)-pad], true
}
