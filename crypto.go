package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const (
	LoginCode         = "eisjI1dHWG4vVTAtNjB0Xw"   // (501,-523)
	LoginKey          = "VUovam5GKndSMHFSSy9kSA==" // (507,-517)
	LoginIV           = "JmM5KnkqNXA9MVVOeC9Ucg==" // (505,-519) & (506,-518)
	PacketSubHashSalt = "QHE0NSNwKWZZQVEhU19xMA=="
	FinishDrawingSalt = "N^&drwseawf"
	// ayo3JnBGQCZVRiEhVjFAQa== (503,-521)
	// OWdCREBKUkl9JjEpQCNuYg== (502,-522)
	// ZiUybmpjc04mNEAkNythbg== (513,-527)
	// WGZSL3JWcUE9L3Q4YipZIQ== (504,-520)
	// 0kg4%dsw (508,-516)
)

var (
	loginKey = []byte{85, 74, 47, 106, 110, 70, 42, 119, 82, 48, 113, 82, 75, 47, 100, 72}
	loginIV  = []byte{38, 99, 57, 42, 121, 42, 53, 112, 61, 49, 85, 78, 120, 47, 84, 114}
)

func decryptLoginString(s string) ([]byte, error) {
	s = strings.TrimRight(s, "=")
	b, err := base64.RawStdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return AESDecrypt(b, loginKey), nil
}

func packetSubHash(sendNum int, packet string) string {
	data := strings.Split(packet, "`")
	packetType := data[0]

	// pop the first element from slice
	copy(data, data[1:])
	data = data[:len(data)-1]

	stringToHash := PacketSubHashSalt + strconv.Itoa(sendNum) + "`" + packetType + "`" + strings.Join(data, "`")
	return GetMD5Hash(stringToHash)[:3]
}

func GetFinishDrawingHash(lvlData string, id string, version string) string {
	return GetMD5Hash(lvlData + id + version + FinishDrawingSalt)
}

func GetMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

func AESEncrypt(src string, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Println("key error1", err)
	}
	if src == "" {
		fmt.Println("plain content empty")
	}
	cbc := cipher.NewCBCEncrypter(block, loginIV)
	content := []byte(src)
	//fmt.Println("content length:", len(content))
	//fmt.Println("block size:", cbc.BlockSize())
	content = ZerosPad(content, cbc.BlockSize())

	crypted := make([]byte, len(content))
	cbc.CryptBlocks(crypted, content)

	return crypted
}

func AESDecrypt(crypt []byte, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Println("key error1", err)
	}
	if len(crypt) == 0 {
		fmt.Println("plain content empty")
	}
	cbc := cipher.NewCBCDecrypter(block, loginIV)
	decrypted := make([]byte, len(crypt))
	cbc.CryptBlocks(decrypted, crypt)

	return ZerosUnpad(decrypted)
}

// Pad the ciphertext with zeros until it is of length blockSize.
func ZerosPad(ciphertext []byte, blockSize int) []byte {
	// determine number of zeros to add
	padLen := blockSize - (len(ciphertext) % blockSize)

	padText := bytes.Repeat([]byte{0}, padLen)
	ciphertext = append(ciphertext, padText...)

	return ciphertext
}

// Trim the trailing zeros from ciphertext.
func ZerosUnpad(ciphertext []byte) []byte {
	return bytes.TrimRight(ciphertext, string(byte(0)))
}
