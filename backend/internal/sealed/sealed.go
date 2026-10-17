// Package sealed keeps a secret typed by the owner (the password of the mail server) in the database: AES-256-GCM under a key derived from the
// master key of the deployment (CODICE_SECRET_KEY), with the purpose bound as associated data, so that what was sealed for one thing does not
// open as another. The key of the wiki provider is sealed the same way by internal/wiki, and the worker opens that one.
package sealed

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// MinMasterKey is the shortest master key accepted: 32 characters, such as `openssl rand -hex 16` makes.
const MinMasterKey = 32

const version = "v1:"

var (
	// ErrNoMasterKey says the deployment has no master key: a secret cannot be kept, nor read.
	ErrNoMasterKey = errors.New("no master key")
	// ErrUnreadable says the sealed secret does not open with this master key and purpose.
	ErrUnreadable = errors.New("the sealed secret does not open with this master key")
)

func gcm(master, purpose string) (cipher.AEAD, error) {
	if len(master) < MinMasterKey {
		return nil, ErrNoMasterKey
	}
	mac := hmac.New(sha256.New, []byte(master))
	mac.Write([]byte(purpose))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal keeps a secret under the master key, for a purpose.
func Seal(master, purpose, secret string) (string, error) {
	aead, err := gcm(master, purpose)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return version + base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(secret), []byte(purpose))), nil
}

// Open reads back what Seal kept.
func Open(master, purpose, sealed string) (string, error) {
	aead, err := gcm(master, purpose)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(sealed, version) {
		return "", ErrUnreadable
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, version))
	if err != nil || len(raw) < aead.NonceSize()+aead.Overhead() {
		return "", ErrUnreadable
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(purpose))
	if err != nil {
		return "", ErrUnreadable
	}
	return string(plain), nil
}
