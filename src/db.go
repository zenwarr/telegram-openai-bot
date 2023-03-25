package src

import (
	"github.com/nutsdb/nutsdb"
	"google.golang.org/protobuf/proto"
	"openai-telegram-bot/src/protos"
	"strconv"
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

func (d *Database) AddDialogMessage(dialogId int64, msg protos.DialogMessage) error {
	return d.db.Update(
		func(tx *nutsdb.Tx) error {
			marshalled, err := proto.Marshal(&msg)
			if err != nil {
				return err
			}

			return tx.RPush("messages", []byte(strconv.FormatInt(dialogId, 10)), marshalled)
		},
	)
}

func (d *Database) GetDialog(dialogId int64) ([]protos.DialogMessage, error) {
	var messages []protos.DialogMessage

	err := d.db.View(
		func(tx *nutsdb.Tx) error {
			entries, err := tx.LRange("messages", []byte(strconv.FormatInt(dialogId, 10)), 0, -1)
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
