package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
)

type Credentials struct {
	Entries map[string]string `json:"credentials"`
}

func GenerateMasterKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func SaveMasterKey(filename string, key []byte) error {
	return os.WriteFile(filename, key, 0600)
}

func LoadMasterKey(filename string) ([]byte, error) {
	key, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func encrypt(data []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCBCEncrypter(block, iv)
	padding := aes.BlockSize - len(data)%aes.BlockSize
	data = append(data, byte(padding)) // PKCS#7 padding
	stream.CryptBlocks(data, data)

	encrypted := append(iv, data...)
	return encrypted, nil
}

func decrypt(encrypted []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(encrypted) < aes.BlockSize {
		return nil, errors.New("encrypted data too short")
	}

	iv := encrypted[:aes.BlockSize]
	encrypted = encrypted[aes.BlockSize:]

	stream := cipher.NewCBCDecrypter(block, iv)
	stream.CryptBlocks(encrypted, encrypted)

	padding := int(encrypted[len(encrypted)-1])
	encrypted = encrypted[:len(encrypted)-padding]

	return encrypted, nil
}

func LoadCredentials(filename string, key []byte) (Credentials, error) {
	var creds Credentials

	encrypted, err := os.ReadFile(filename)
	if err != nil {
		return creds, err
	}

	decrypted, err := decrypt(encrypted, key)
	if err != nil {
		return creds, err
	}

	err = json.Unmarshal(decrypted, &creds)
	if err != nil {
		return creds, err
	}

	return creds, nil
}

func saveCredentials(filename string, creds Credentials, key []byte) error {
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return err
	}

	encryptedData, err := encrypt(credsJSON, key)
	if err != nil {
		return err
	}

	err = os.WriteFile(filename, encryptedData, 0600)
	if err != nil {
		return err
	}

	return nil
}

func UpsertCredentials(filename string, hostname string, password string, key []byte) error {
	creds, err := LoadCredentials(filename, key)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if creds.Entries == nil {
		creds.Entries = make(map[string]string)
	}

	creds.Entries[hostname] = password

	return saveCredentials(filename, creds, key)
}

func RemoveCredentials(filename string, hostname string, key []byte) error {
	creds, err := LoadCredentials(filename, key)
	if err != nil {
		return err
	}

	delete(creds.Entries, hostname)

	return saveCredentials(filename, creds, key)
}
