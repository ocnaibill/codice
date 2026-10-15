package wiki

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

// The key of the provider is typed by the owner and kept sealed in the database (DEC-185, section 17.2): AES-256-GCM under a key
// derived from the master key of the deployment (CODICE_SECRET_KEY), with the purpose bound as associated data. The worker, which
// calls the model, opens it the same way (worker/wiki_secret.py); the format is "v1:" and the base64 of nonce, ciphertext and tag.

// MinMasterKey is the shortest master key accepted: 32 characters, such as `openssl rand -hex 16` makes.
const MinMasterKey = 32

const (
	sealVersion = "v1:"
	sealPurpose = "codice:wiki-provider-key"
)

var (
	// ErrNoMasterKey says the deployment has no master key: a key cannot be kept, nor read.
	ErrNoMasterKey = errors.New("no master key")
	// ErrUnreadable says the sealed key does not open with this master key (another deployment's, or the master key changed).
	ErrUnreadable = errors.New("the sealed key does not open with this master key")
)

func sealKey(master string) ([]byte, error) {
	if len(master) < MinMasterKey {
		return nil, ErrNoMasterKey
	}
	mac := hmac.New(sha256.New, []byte(master))
	mac.Write([]byte(sealPurpose))
	return mac.Sum(nil), nil
}

func gcm(master string) (cipher.AEAD, error) {
	key, err := sealKey(master)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal keeps a secret under the master key.
func Seal(master, secret string) (string, error) {
	aead, err := gcm(master)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := aead.Seal(nonce, nonce, []byte(secret), []byte(sealPurpose))
	return sealVersion + base64.StdEncoding.EncodeToString(out), nil
}

// Open reads back what Seal kept.
func Open(master, sealed string) (string, error) {
	aead, err := gcm(master)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(sealed, sealVersion) {
		return "", ErrUnreadable
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, sealVersion))
	if err != nil || len(raw) < aead.NonceSize()+aead.Overhead() {
		return "", ErrUnreadable
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(sealPurpose))
	if err != nil {
		return "", ErrUnreadable
	}
	return string(plain), nil
}
