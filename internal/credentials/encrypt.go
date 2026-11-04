package credentials

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Entry struct {
	Password string `json:"password"`
	Hostname string `json:"hostname"`
	User     string `json:"user"`
	Port     string `json:"port"`
}

type Credentials struct {
	Entries map[string]Entry `json:"credentials"`
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

// Files without this prefix use the legacy AES-CBC format.
var gcmMagic = []byte("s1h-gcm1")

func encrypt(data []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := append(append([]byte{}, gcmMagic...), nonce...)
	return gcm.Seal(out, nonce, data, gcmMagic), nil
}

func decrypt(encrypted []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	encrypted = encrypted[len(gcmMagic):]
	if len(encrypted) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("credentials file is truncated")
	}
	nonce, sealed := encrypted[:gcm.NonceSize()], encrypted[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, gcmMagic)
	if err != nil {
		return nil, errors.New("credentials file is corrupted or was encrypted with another key")
	}
	return plain, nil
}

func decryptLegacyCBC(encrypted []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(encrypted) < 2*aes.BlockSize || len(encrypted)%aes.BlockSize != 0 {
		return nil, errors.New("credentials file is truncated or corrupted")
	}
	iv, data := encrypted[:aes.BlockSize], append([]byte{}, encrypted[aes.BlockSize:]...)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(data, data)

	n := bytes.LastIndexByte(data, '}')
	if n == -1 {
		return nil, errors.New("credentials file is corrupted or was encrypted with another key")
	}
	return data[:n+1], nil
}

func LoadCredentials(filename string, key []byte) (Credentials, error) {
	var creds Credentials

	encrypted, err := os.ReadFile(filename)
	if err != nil {
		return creds, err
	}

	legacy := !bytes.HasPrefix(encrypted, gcmMagic)
	var decrypted []byte
	if legacy {
		decrypted, err = decryptLegacyCBC(encrypted, key)
	} else {
		decrypted, err = decrypt(encrypted, key)
	}
	if err != nil {
		return creds, err
	}

	if err := json.Unmarshal(decrypted, &creds); err != nil {
		return creds, fmt.Errorf("credentials file is corrupted: %w", err)
	}

	if legacy {
		_ = saveCredentials(filename, creds, key)
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

	tmp, err := os.CreateTemp(filepath.Dir(filename), ".credentials-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(encryptedData); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filename)
}

func UpsertCredential(filename string, host, hostname, user, port string, password string, key []byte) error {
	creds, err := LoadCredentials(filename, key)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if creds.Entries == nil {
		creds.Entries = make(map[string]Entry)
	}

	entry := Entry{
		Password: password,
	}
	if hostname != "" {
		entry.Hostname = hostname
		entry.User = user
		entry.Port = port
	}
	creds.Entries[host] = entry

	return saveCredentials(filename, creds, key)
}

func RemoveCredential(filename string, host string, key []byte) error {
	creds, err := LoadCredentials(filename, key)
	if err != nil {
		return err
	}

	delete(creds.Entries, host)

	return saveCredentials(filename, creds, key)
}

func RevealCredentialPassword(filename string, host string, key []byte) (string, error) {
	creds, err := LoadCredentials(filename, key)
	if err != nil {
		return "", err
	}

	cred, has := creds.Entries[host]
	if !has {
		return "", fmt.Errorf("no entry for %s", host)
	}

	return cred.Password, nil
}

func SetPassword(filename string, host string, password string, key []byte) error {
	creds, err := LoadCredentials(filename, key)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if creds.Entries == nil {
		creds.Entries = make(map[string]Entry)
	}

	entry := creds.Entries[host]
	entry.Password = password
	if entry.Password == "" && entry.Hostname == "" {
		delete(creds.Entries, host)
	} else {
		creds.Entries[host] = entry
	}

	return saveCredentials(filename, creds, key)
}

func PutEntry(filename string, host string, entry Entry, key []byte) error {
	creds, err := LoadCredentials(filename, key)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if creds.Entries == nil {
		creds.Entries = make(map[string]Entry)
	}
	creds.Entries[host] = entry
	return saveCredentials(filename, creds, key)
}
