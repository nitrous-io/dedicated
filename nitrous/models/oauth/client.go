package oauth

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Client struct {
	ID          int64  `db:"id" json:"id"`
	UserID      int64  `db:"user_id" json:"user_id"`
	Name        string `db:"name" json:"name"`
	Description string `db:"description" json:"description"`
	ClientID    string `db:"client_id" json:"client_id"`
	RedirectURI string `db:"redirect_uri" json:"redirect_uri"`
}

func FindClientByID(d *db.Database, clientID string) (*Client, error) {
	client := &Client{}

	err := d.Get(client, `SELECT FROM oauth_clients WHERE client_id=$1;`, clientID)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func FindClientWithSecret(d *db.Database, clientID, secret string) (*Client, error) {
	client := &Client{}

	err := d.Get(client, `SELECT FROM oauth_clients WHERE client_id=$1 AND secret=$2;`, clientID, secret)
	if err != nil {
		return nil, err
	}

	return client, nil
}
