package cluster

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sync"
	"unicode/utf8"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
)

// EffectSecrets stores only authenticated ciphertext in FS_OBJECTS. Keys are
// supplied by versioned Swarm secrets at candidate runtime construction.
type EffectObjectStore interface {
	GetBytes(string, ...nats.GetObjectOpt) ([]byte, error)
	PutBytes(string, []byte, ...nats.ObjectOpt) (*nats.ObjectInfo, error)
	List(...nats.ListObjectsOpt) ([]*nats.ObjectInfo, error)
	Delete(string) error
}

type EffectSecrets struct {
	Objects     EffectObjectStore
	ActiveKeyID string
	Keys        map[string][]byte
}

// LoadEffectSecrets reads raw 32-byte versioned Swarm secrets. The active key
// is required for new objects; older keys remain available for reads and
// backup retention until their objects have been collected.
func LoadEffectSecrets(objects EffectObjectStore, activeID string, paths map[string]string) (EffectSecrets, error) {
	if objects == nil || activeID == "" || len(paths) == 0 {
		return EffectSecrets{}, fmt.Errorf("effect secret configuration incomplete")
	}
	keys := make(map[string][]byte, len(paths))
	for id, path := range paths {
		if id == "" || path == "" {
			return EffectSecrets{}, fmt.Errorf("invalid effect secret key reference")
		}
		key, err := os.ReadFile(path)
		if err != nil {
			return EffectSecrets{}, err
		}
		if len(key) != 32 {
			return EffectSecrets{}, fmt.Errorf("effect secret key %s must be 32 bytes", id)
		}
		keys[id] = key
	}
	if len(keys[activeID]) != 32 {
		return EffectSecrets{}, fmt.Errorf("active effect secret key unavailable")
	}
	return EffectSecrets{Objects: objects, ActiveKeyID: activeID, Keys: keys}, nil
}

var effectStageLocks [256]sync.Mutex

func effectAAD(commandID, targetCID string) []byte {
	return []byte("fs-effect-v1\x00" + commandID + "\x00" + targetCID)
}

func (s EffectSecrets) key(id string) (cipher.AEAD, error) {
	key := s.Keys[id]
	if id == "" || len(key) != 32 {
		return nil, fmt.Errorf("effect encryption key unavailable")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s EffectSecrets) StagePrivateMessage(commandID, targetCID, recipient, body string) (*pb.PrivateMessageEffect, error) {
	if s.Objects == nil || !canonicalUUID(commandID) || targetCID == "" || recipient == "" || body == "" || !utf8.ValidString(body) {
		return nil, fmt.Errorf("invalid private message")
	}
	stripe := sha256.Sum256([]byte(commandID))[0]
	effectStageLocks[stripe].Lock()
	defer effectStageLocks[stripe].Unlock()
	name := "effect/" + commandID
	if existing, err := s.Objects.GetBytes(name); err == nil {
		value := &pb.ObjectValue{}
		if pb.UnmarshalStrict(existing, value) != nil || value.GetEffectSecret() == nil {
			return nil, fmt.Errorf("existing effect object is invalid")
		}
		plain, err := s.OpenPrivateMessage(commandID, targetCID, name, digest(existing))
		if err != nil || plain != body {
			return nil, fmt.Errorf("effect object command ID collision")
		}
		return &pb.PrivateMessageEffect{Recipient: recipient, ObjectName: name, Sha256: digest(existing)}, nil
	} else if !errors.Is(err, nats.ErrObjectNotFound) {
		return nil, err
	}
	aead, err := s.key(s.ActiveKeyID)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	value := &pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_EffectSecret{EffectSecret: &pb.EffectSecret{
		CommandId: commandID, KeyId: s.ActiveKeyID, Nonce: nonce,
		Ciphertext: aead.Seal(nil, nonce, []byte(body), effectAAD(commandID, targetCID))}}}
	data, err := objectBytes(value)
	if err != nil {
		return nil, err
	}
	if _, err := s.Objects.PutBytes(name, data); err != nil {
		return nil, err
	}
	readback, err := s.Objects.GetBytes(name)
	if err != nil || !bytes.Equal(readback, data) {
		return nil, fmt.Errorf("effect object verification failed")
	}
	return &pb.PrivateMessageEffect{Recipient: recipient, ObjectName: name, Sha256: digest(data)}, nil
}

func (s EffectSecrets) OpenPrivateMessage(commandID, targetCID, name, sha string) (string, error) {
	if s.Objects == nil || !canonicalUUID(commandID) || targetCID == "" || name != "effect/"+commandID || len(sha) != 64 {
		return "", fmt.Errorf("invalid effect object reference")
	}
	data, err := s.Objects.GetBytes(name)
	if err != nil {
		return "", err
	}
	if digest(data) != sha || len(data) > MaxObjectBytes {
		return "", fmt.Errorf("effect object checksum mismatch")
	}
	value := &pb.ObjectValue{}
	if err := pb.UnmarshalStrict(data, value); err != nil || value.SchemaVersion != 1 || value.GetEffectSecret() == nil {
		return "", fmt.Errorf("invalid effect object")
	}
	secret := value.GetEffectSecret()
	if secret.CommandId != commandID || len(secret.Nonce) != 12 {
		return "", fmt.Errorf("effect object identity mismatch")
	}
	aead, err := s.key(secret.KeyId)
	if err != nil {
		return "", err
	}
	plain, err := aead.Open(nil, secret.Nonce, secret.Ciphertext, effectAAD(commandID, targetCID))
	if err != nil || !utf8.Valid(plain) {
		return "", fmt.Errorf("effect object authentication failed")
	}
	return string(plain), nil
}
