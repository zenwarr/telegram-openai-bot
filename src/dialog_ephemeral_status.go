package src

import "sync"

var (
	dialogEphemeralStatus = struct {
		sync.RWMutex
		m map[string]bool
	}{m: make(map[string]bool)}
)

func SetDialogEphemeralStatus(dialogId string, value bool) {
	dialogEphemeralStatus.Lock()
	defer dialogEphemeralStatus.Unlock()

	if value {
		dialogEphemeralStatus.m[dialogId] = value
	} else {
		delete(dialogEphemeralStatus.m, dialogId)
	}
}

func GetDialogEphemeralStatus(dialogId string) bool {
	dialogEphemeralStatus.RLock()
	defer dialogEphemeralStatus.RUnlock()

	return dialogEphemeralStatus.m[dialogId]
}
