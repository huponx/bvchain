package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"io"
)

// EncryptCryptoJS encrypts data using the same algorithm as CryptoJS.AES.encrypt(data, passphrase).
//
// When CryptoJS receives a string key (not a WordArray), it:
//  1. Generates a random 8-byte salt
//  2. Derives a 32-byte key and 16-byte IV using OpenSSL EVP_BytesToKey (MD5, 1 iteration)
//  3. Encrypts with AES-256-CBC + PKCS7 padding
//  4. Returns Base64("Salted__" + salt + ciphertext)
func EncryptCryptoJS(data, passphrase string) (string, error) {
	salt := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}

	key, iv := evpBytesToKey([]byte(passphrase), salt, 32, 16)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	padded := pkcs7Pad([]byte(data), aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	// OpenSSL/CryptoJS wire format: "Salted__" (8 bytes) + salt (8 bytes) + ciphertext
	var buf bytes.Buffer
	buf.WriteString("Salted__")
	buf.Write(salt)
	buf.Write(ciphertext)

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// evpBytesToKey implements OpenSSL EVP_BytesToKey with MD5 and a single iteration.
// It produces keyLen+ivLen bytes of key material by repeatedly hashing:
//
//	D_0 = MD5(password || salt)
//	D_i = MD5(D_{i-1} || password || salt)
func evpBytesToKey(password, salt []byte, keyLen, ivLen int) (key, iv []byte) {
	needed := keyLen + ivLen
	var material []byte
	var prev []byte
	for len(material) < needed {
		h := md5.New()
		h.Write(prev)
		h.Write(password)
		h.Write(salt)
		prev = h.Sum(nil)
		material = append(material, prev...)
	}
	return material[:keyLen], material[keyLen : keyLen+ivLen]
}

// pkcs7Pad pads data to a multiple of blockSize using PKCS#7.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
}
