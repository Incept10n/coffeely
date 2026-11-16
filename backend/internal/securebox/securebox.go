// Package securebox — обратимое шифрование паролей рабочих аккаунтов бариста.
//
// Пароль хранится двумя способами: argon2id-хеш в users.password_hash (проверка
// входа) и AES-256-GCM в users.password_enc (показ владельцу в кабинете).
// Это осознанное продуктовое решение: владелец кофейни должен видеть пароль
// баристы после перезагрузки страницы и менять его. Ключ выводится из
// JWT_SECRET через HMAC-SHA256 с отдельной меткой домена, поэтому нового
// секрета в проде не нужно; при смене JWT_SECRET старые password_enc перестают
// расшифровываться — владелец просто задаёт пароль заново.
package securebox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// keyInfo — метка домена: тот же JWT_SECRET не даёт ключ для других целей.
const keyInfo = "coffeely/user-password/v1"

// Box шифрует и расшифровывает короткие секреты (пароли бариста).
type Box struct {
	aead cipher.AEAD
}

// New выводит ключ из секрета приложения. 32-байтный ключ для AES-256
// получается детерминированно, поэтому ошибки конфигурации здесь невозможны.
func New(secret string) *Box {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(keyInfo))
	key := mac.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Sprintf("securebox: aes: %v", err))
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Sprintf("securebox: gcm: %v", err))
	}
	return &Box{aead: aead}
}

// Encrypt возвращает nonce||ciphertext||tag.
func (b *Box) Encrypt(plain string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("securebox: nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, []byte(plain), nil), nil
}

// Decrypt расшифровывает результат Encrypt.
func (b *Box) Decrypt(data []byte) (string, error) {
	nonceSize := b.aead.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("securebox: слишком короткий шифротекст")
	}
	plain, err := b.aead.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("securebox: decrypt: %w", err)
	}
	return string(plain), nil
}
