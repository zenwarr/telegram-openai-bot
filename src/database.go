package src

import (
	"encoding/binary"
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

func (d *Database) DeleteDialog(dialogId string) error {
	return d.db.Update(
		func(tx *nutsdb.Tx) error {
			err := tx.LTrim("messages", []byte(dialogId), 0, -1)
			if err != nil && !nutsdb.IsBucketNotFound(err) {
				return err
			}

			return nil
		},
	)
}

func (d *Database) DecimateDialog(dialogId string) error {
	// delete first half of the messages
	return d.db.Update(
		func(tx *nutsdb.Tx) error {
			key := []byte(dialogId)

			entryCount, err := tx.LSize("messages", key)
			if err != nil {
				return err
			}

			err = tx.LTrim("messages", key, entryCount/2, -1)
			if err != nil {
				return err
			}

			return nil
		},
	)
}

func (d *Database) ReplaceDialog(dialogId string, msg *protos.DialogMessage) error {
	return d.db.Update(
		func(tx *nutsdb.Tx) error {
			key := []byte(dialogId)

			err := tx.LTrim("messages", key, 0, -1)
			if err != nil {
				return err
			}

			marshalled, err := proto.Marshal(msg)
			if err != nil {
				return err
			}

			err = tx.RPush("messages", key, marshalled)
			if err != nil {
				return err
			}

			return nil
		},
	)
}

func (d *Database) GetNotWantedSent(userId int64) (bool, error) {
	var notWantedSent bool

	err := d.db.View(
		func(tx *nutsdb.Tx) error {
			entry, err := tx.Get("not_wanted_sent", intToBytes(userId))
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
			return tx.Put("not_wanted_sent", intToBytes(userId), []byte{value}, uint32((time.Hour * 24 * 7).Seconds()))
		},
	)
}

func (d *Database) GetLastInteractionTime(dialogId string) (time.Time, error) {
	var lastInteractionTime time.Time

	err := d.db.View(
		func(tx *nutsdb.Tx) error {
			entry, err := tx.Get("last_interaction_time", []byte(dialogId))
			if nutsdb.IsBucketNotFound(err) {
				return nil
			}

			if err != nil {
				return err
			}

			lastInteractionTime = time.Unix(int64(binary.BigEndian.Uint64(entry.Value)), 0)

			return nil
		},
	)
	if err != nil {
		return lastInteractionTime, err
	}

	return lastInteractionTime, nil
}

const DialogStateNone = 0
const DialogStateContextLimit = 1

func (d *Database) SetDialogState(dialogId string, state int64) error {
	return d.db.Update(
		func(tx *nutsdb.Tx) error {
			if state == DialogStateNone {
				return tx.Delete("dialog_state", []byte(dialogId))
			}

			return tx.Put("dialog_state", []byte(dialogId), intToBytes(state), uint32((time.Hour * 24 * 7).Seconds()))
		},
	)
}

func (d *Database) GetDialogState(dialogId string) (int64, error) {
	var state int64

	err := d.db.View(
		func(tx *nutsdb.Tx) error {
			entry, err := tx.Get("dialog_state", []byte(dialogId))
			if nutsdb.IsBucketNotFound(err) || err.Error() == "key not found in the bucket" {
				return nil
			}

			if err != nil {
				return err
			}

			state = bytesToInt(entry.Value)

			return nil
		},
	)
	if err != nil {
		return 0, err
	}

	return state, nil
}

func intToBytes(i int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(i))
	return b
}

func bytesToInt(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}
