package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

// RSAKeyPair RSA 密钥对（PEM 格式）。
type RSAKeyPair struct {
	PrivateKey string // PKCS8 PEM
	PublicKey  string // PKIX  PEM
}

// GenerateRSAKeyPair 生成 2048 位 RSA 密钥对。
func GenerateRSAKeyPair() (*RSAKeyPair, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("rsa: generate key: %w", err)
	}

	privPEM, err := marshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	pubPEM, err := marshalPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, err
	}
	return &RSAKeyPair{PrivateKey: privPEM, PublicKey: pubPEM}, nil
}

// Encrypt 用公钥加密，返回 base64 编码密文（PKCS1v15）。
func Encrypt(publicKeyPEM string, data []byte) (string, error) {
	pub, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return "", err
	}
	cipher, err := rsa.EncryptPKCS1v15(rand.Reader, pub, data)
	if err != nil {
		return "", fmt.Errorf("rsa: encrypt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(cipher), nil
}

// Decrypt 用私钥解密 base64 编码密文，返回明文字节。
func Decrypt(privateKeyPEM, ciphertextB64 string) ([]byte, error) {
	priv, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("rsa: base64 decode: %w", err)
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, priv, raw)
	if err != nil {
		return nil, fmt.Errorf("rsa: decrypt: %w", err)
	}
	return plain, nil
}

// DecryptToString 用私钥解密并返回字符串（API Key 签名验证专用）。
func DecryptToString(privateKeyPEM, ciphertextB64 string) (string, error) {
	b, err := Decrypt(privateKeyPEM, ciphertextB64)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ── 内部辅助 ──────────────────────────────────────────────────

func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("rsa: invalid PEM")
	}
	switch block.Type {
	case "PRIVATE KEY": // PKCS8
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("rsa: parse PKCS8: %w", err)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("rsa: not an RSA private key")
		}
		return rsaKey, nil
	case "RSA PRIVATE KEY": // PKCS1
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("rsa: unsupported PEM type %q", block.Type)
	}
}

func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("rsa: invalid PEM")
	}
	switch block.Type {
	case "PUBLIC KEY": // PKIX
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("rsa: parse PKIX: %w", err)
		}
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("rsa: not an RSA public key")
		}
		return rsaKey, nil
	case "RSA PUBLIC KEY": // PKCS1
		return x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		return nil, fmt.Errorf("rsa: unsupported PEM type %q", block.Type)
	}
}

func marshalPrivateKey(key *rsa.PrivateKey) (string, error) {
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("rsa: marshal private key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b})), nil
}

func marshalPublicKey(key *rsa.PublicKey) (string, error) {
	b, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", fmt.Errorf("rsa: marshal public key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: b})), nil
}
