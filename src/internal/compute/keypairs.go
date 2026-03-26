package compute

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"golang.org/x/crypto/ssh"
)

// KeyPair is a simplified representation of a Nova keypair.
type KeyPair struct {
	Name string
	Type string
}

// ListKeyPairs fetches all keypairs.
func ListKeyPairs(ctx context.Context, client *gophercloud.ServiceClient) ([]KeyPair, error) {
	var result []KeyPair
	err := keypairs.List(client, keypairs.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := keypairs.ExtractKeyPairs(page)
		if err != nil {
			return false, err
		}
		for _, kp := range extracted {
			result = append(result, KeyPair{
				Name: kp.Name,
				Type: kp.Type,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing keypairs: %w", err)
	}
	return result, nil
}

// KeyPairFull includes the private key (only populated on create/generate).
type KeyPairFull struct {
	Name       string
	Type       string
	PublicKey  string
	PrivateKey string
}

// GenerateAndImportKeyPair generates a keypair locally and imports the public key.
// algorithm is "rsa" or "ed25519". keySize is only used for RSA (e.g. 2048, 4096).
func GenerateAndImportKeyPair(ctx context.Context, client *gophercloud.ServiceClient, name, algorithm string, keySize int) (*KeyPairFull, error) {
	var pubKeyBytes []byte
	var privKeyPEM string

	switch algorithm {
	case "ed25519":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generating ed25519 key: %w", err)
		}
		sshPub, err := ssh.NewPublicKey(pub)
		if err != nil {
			return nil, fmt.Errorf("converting ed25519 public key: %w", err)
		}
		pubKeyBytes = ssh.MarshalAuthorizedKey(sshPub)

		privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, fmt.Errorf("marshaling ed25519 private key: %w", err)
		}
		privKeyPEM = string(pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privBytes,
		}))

	default: // rsa
		if keySize == 0 {
			keySize = 4096
		}
		privKey, err := rsa.GenerateKey(rand.Reader, keySize)
		if err != nil {
			return nil, fmt.Errorf("generating rsa key (%d bits): %w", keySize, err)
		}
		sshPub, err := ssh.NewPublicKey(&privKey.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("converting rsa public key: %w", err)
		}
		pubKeyBytes = ssh.MarshalAuthorizedKey(sshPub)

		privKeyPEM = string(pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privKey),
		}))
	}

	pubKeyStr := string(pubKeyBytes)

	// Import via Nova
	kp, err := ImportKeyPair(ctx, client, name, pubKeyStr)
	if err != nil {
		return nil, err
	}
	kp.PrivateKey = privKeyPEM
	return kp, nil
}

// ImportKeyPair imports an existing public key.
func ImportKeyPair(ctx context.Context, client *gophercloud.ServiceClient, name, publicKey string) (*KeyPairFull, error) {
	opts := keypairs.CreateOpts{
		Name:      name,
		PublicKey: publicKey,
	}
	client.Microversion = "2.2"
	kp, err := keypairs.Create(ctx, client, opts).Extract()
	client.Microversion = ""
	if err != nil {
		return nil, fmt.Errorf("importing keypair %s: %w", name, err)
	}
	return &KeyPairFull{
		Name:      kp.Name,
		Type:      kp.Type,
		PublicKey: kp.PublicKey,
	}, nil
}

// GetKeyPair fetches a single keypair by name.
func GetKeyPair(ctx context.Context, client *gophercloud.ServiceClient, name string) (*KeyPairFull, error) {
	kp, err := keypairs.Get(ctx, client, name, keypairs.GetOpts{}).Extract()
	if err != nil {
		return nil, fmt.Errorf("getting keypair %s: %w", name, err)
	}
	return &KeyPairFull{
		Name:      kp.Name,
		Type:      kp.Type,
		PublicKey: kp.PublicKey,
	}, nil
}

// DeleteKeyPair deletes a keypair by name.
func DeleteKeyPair(ctx context.Context, client *gophercloud.ServiceClient, name string) error {
	r := keypairs.Delete(ctx, client, name, keypairs.DeleteOpts{})
	if r.Err != nil {
		return fmt.Errorf("deleting keypair %s: %w", name, r.Err)
	}
	return nil
}
