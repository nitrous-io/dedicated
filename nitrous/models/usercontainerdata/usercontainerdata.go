package usercontainerdata

import (
	"time"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type UserContainerData struct {
	ID          int64         `db:"id"`
	UserID      int64         `db:"user_id"`
	ContainerID int64         `db:"container_id"`
	Metadata    hstore.Hstore `db:"metadata"`
	CreatedAt   *time.Time    `db:"created_at"`
	UpdatedAt   *time.Time    `db:"updated_at"`
}

func Find(db *db.Database, userID, cID int64) (*UserContainerData, error) {
	record := &UserContainerData{}
	err := db.Get(record,
		`SELECT * FROM user_container_data
                WHERE user_id = $1 AND container_id = $2;`, userID, cID)
	if err != nil {
		return nil, err
	}
	return record, err
}

func SetData(db *db.Database, userID, cID int64, key, value string) error {
	_, err := db.Exec(`WITH upsert AS 
                (UPDATE user_container_data 
                        SET metadata = metadata || hstore($1, $2), updated_at = now() 
                        WHERE user_id = $3 AND container_id = $4 RETURNING *) 
                INSERT INTO user_container_data (user_id, container_id, metadata) 
                        SELECT $3, $4, hstore($1, $2) WHERE NOT EXISTS (SELECT * FROM upsert);`,
		key, value, userID, cID)

	return err
}
