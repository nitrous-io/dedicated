package keypairs

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"time"

	"code.google.com/p/go.crypto/ssh"

	"github.com/nitrous-io/gorainbow/cipher"
	"github.com/nitrous-io/nitrous/db"
)

var (
	KeyLen = 2048
)

type Keypair struct {
	ID                    int64      `db:"id"`
	OwnerType             string     `db:"owner_type"`
	OwnerID               int64      `db:"owner_id"`
	PublicKey             string     `db:"public_key"`
	EncryptedPrivateKey   string     `db:"encrypted_private_key"`
	EncryptedPrivateKeyIv string     `db:"encrypted_private_key_iv"`
	CreatedAt             *time.Time `db:"created_at"`
	UpdatedAt             *time.Time `db:"updated_at"`
}

// Accepts a private key for specified owner and save it as a keypair
func Create(
	db db.QueryReceiver,
	priv *rsa.PrivateKey,
	aesKey string,
	ownerType string,
	ownerID int64,
) (*Keypair, error) {
	kp, err := generateKeypair(priv, aesKey)
	if err != nil {
		return nil, err
	}

	kp.OwnerType = ownerType
	kp.OwnerID = ownerID

	stmt, err := db.PrepareNamed(`INSERT INTO key_pairs
	(owner_type, owner_id, public_key, encrypted_private_key, encrypted_private_key_iv)
	VALUES (:owner_type, :owner_id, :public_key, :encrypted_private_key, :encrypted_private_key_iv)
	RETURNING *;`)
	if err != nil {
		return nil, err
	}
	if err := stmt.QueryRowx(kp).StructScan(kp); err != nil {
		return nil, err
	}
	return kp, nil
}

func Get(db db.QueryReceiver, ownerType string, ownerID int64) (*Keypair, error) {
	kp := &Keypair{}
	q := `SELECT * FROM key_pairs WHERE owner_type = $1 AND owner_id = $2`
	err := db.Get(kp, q, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	return kp, nil
}

// wrapper around `rsa.GenerateKey` to return a private key with predefined key
// length
func GenerateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, KeyLen)
}

// return pem format of the public key
func PublicKey(pub *rsa.PublicKey) ([]byte, error) {
	data, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return ssh.MarshalAuthorizedKey(data), nil
}

// return pem format of the private key
func privateKey(priv *rsa.PrivateKey) []byte {
	// ASN.1 DER encoded form
	privder := x509.MarshalPKCS1PrivateKey(priv)
	privblk := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privder,
	}
	return pem.EncodeToMemory(&privblk)
}

// generate a new keypair
func generateKeypair(priv *rsa.PrivateKey, aesKey string) (*Keypair, error) {
	// get the generated public key
	pubkey, err := PublicKey(&priv.PublicKey)
	if err != nil {
		return nil, err
	}
	// get the encrypted private key
	privPem := privateKey(priv)
	iv, privKey, err := cipher.Encrypt([]byte(aesKey), privPem)
	if err != nil {
		return nil, err
	}
	kp := &Keypair{
		PublicKey:             string(pubkey),
		EncryptedPrivateKey:   string(privKey),
		EncryptedPrivateKeyIv: string(iv),
	}
	return kp, nil
}
