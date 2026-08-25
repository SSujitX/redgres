package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var argon2Mu sync.Mutex

const (
	AlgorithmArgon2id  = "argon2id"
	DefaultMemoryKiB   = 64 * 1024
	DefaultTime        = 3
	DefaultParallelism = 4
	DefaultSaltLen     = 16
	DefaultKeyLen      = 32

	MinPasswordRunes = 15
	MaxPasswordBytes = 1024
	MaxUsernameRunes = 64
)

var (
	ErrInvalidHash      = errors.New("invalid password hash")
	ErrMismatchedHash   = errors.New("password does not match")
	ErrWeakPassword     = errors.New("password does not meet the minimum strength policy")
	ErrPasswordTooLong  = errors.New("password exceeds the maximum length")
	ErrInvalidUsername  = errors.New("username is invalid")
	ErrOwnerExists      = errors.New("owner already exists")
	ErrPasswordConfirm  = errors.New("passwords do not match")
	ErrPasswordRequired = errors.New("password is required")
)

var dummyEncodedHash string

func init() {
	hash, err := Hash("redgres-dummy-password-not-used")
	if err != nil {
		panic(err)
	}
	dummyEncodedHash = hash
}

type Params struct {
	Memory      uint32
	Time        uint32
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

func DefaultParams() Params {
	return Params{
		Memory:      DefaultMemoryKiB,
		Time:        DefaultTime,
		Parallelism: DefaultParallelism,
		SaltLen:     DefaultSaltLen,
		KeyLen:      DefaultKeyLen,
	}
}

func DummyHash() string {
	return dummyEncodedHash
}

func Hash(password string) (string, error) {
	return HashWithParams(password, DefaultParams())
}

func HashWithParams(password string, p Params) (string, error) {
	if p.Memory == 0 || p.Time == 0 || p.Parallelism == 0 || p.SaltLen == 0 || p.KeyLen == 0 {
		return "", errors.New("argon2 parameters must be non-zero")
	}
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2IDKey([]byte(password), salt, p.Time, p.Memory, p.Parallelism, p.KeyLen)
	return encode(p, salt, sum), nil
}

func Verify(encoded, password string) error {
	p, salt, sum, err := decode(encoded)
	if err != nil {
		return err
	}
	computed := argon2IDKey([]byte(password), salt, p.Time, p.Memory, p.Parallelism, uint32(len(sum)))
	if subtle.ConstantTimeCompare(sum, computed) != 1 {
		return ErrMismatchedHash
	}
	return nil
}

func VerifyUnknown(password string) {
	_ = Verify(dummyEncodedHash, password)
}

func ValidatePassword(password, username string) error {
	if strings.TrimSpace(password) == "" {
		return ErrWeakPassword
	}
	if len(password) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	if utf8.RuneCountInString(password) < MinPasswordRunes {
		return ErrWeakPassword
	}
	if NormalizeUsername(username) != "" && NormalizeUsername(password) == NormalizeUsername(username) {
		return ErrWeakPassword
	}
	return nil
}

func argon2IDKey(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
	argon2Mu.Lock()
	defer argon2Mu.Unlock()
	return argon2.IDKey(password, salt, time, memory, threads, keyLen)
}

func encode(p Params, salt, sum []byte) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		AlgorithmArgon2id,
		argon2.Version,
		p.Memory,
		p.Time,
		p.Parallelism,
		b64.EncodeToString(salt),
		b64.EncodeToString(sum),
	)
}

func decode(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if parts[1] != AlgorithmArgon2id {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return Params{}, nil, nil, ErrInvalidHash
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Parallelism); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	sum, err := b64.DecodeString(parts[5])
	if err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	p.SaltLen = uint32(len(salt))
	p.KeyLen = uint32(len(sum))
	return p, salt, sum, nil
}
