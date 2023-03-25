package src

import (
	"github.com/nutsdb/nutsdb"
	"google.golang.org/protobuf/proto"
	"openai-telegram-bot/src/protos"
	"time"
)

type Database struct {
	db *nutsdb.DB
}

func NewDatabase(path string) (*Database, error) {
	db, err := nutsdb.Open(
		nutsdb.DefaultOptions,
		nutsdb.WithDir(path),
	)
	if err != nil {
		return nil, err
	}

	return &Database{
		db,
	}, nil
}

func (d *Database) Close() error {
	return d.db.Close()
}

func (d *Database) AddDialogMessage(dialogId string, msg protos.DialogMessage) error {
	return d.db.Update(
		func(tx *nutsdb.Tx) error {
			marshalled, err := proto.Marshal(&msg)
			if err != nil {
				return err
			}

			return tx.RPush("messages", []byte(dialogId), marshalled)
		},
	)
}

func (d *Database) GetDialog(dialogId string) ([]protos.DialogMessage, error) {
	var messages []protos.DialogMessage

	err := d.db.View(
		func(tx *nutsdb.Tx) error {
			entries, err := tx.LRange("messages", []byte(dialogId), 0, -1)
			if err != nil {
				return err
			}

			for _, entry := range entries {
				msg := &protos.DialogMessage{}
				err := proto.Unmarshal(entry, msg)
				if err != nil {
					return err
				}

				messages = append(messages, *msg)
			}

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

func (d *Database) GetNotWantedSent(userId int64) (bool, error) {
	var notWantedSent bool

	err := d.db.View(
		func(tx *nutsdb.Tx) error {
			entry, err := tx.Get("not_wanted_sent", []byte(string(userId)))
			if nutsdb.IsBucketNotFound(err) {
				return nil
			}

			if err != nil {
				return err
			}

			notWantedSent = entry.Value[0] == 1

			return nil
		},
	)
	if err != nil {
		return false, err
	}

	return notWantedSent, nil
}

func (d *Database) SetNotWantedSent(userId int64) error {
	return d.db.Update(
		func(tx *nutsdb.Tx) error {
			var value byte = 1
			return tx.Put("not_wanted_sent", []byte(string(userId)), []byte{value}, uint32((time.Hour * 24 * 7).Seconds()))
		},
	)
}
