package mailer

import (
	"context"
	"sync"
)

type CancellableContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

type MailCanceller struct {
	Mutex     *sync.Mutex
	CancelMap map[string]CancellableContext
}

func NewMailCanceller() *MailCanceller {
	m := make(map[string]CancellableContext)
	return &MailCanceller{Mutex: &sync.Mutex{}, CancelMap: m}
}

func (mc *MailCanceller) ContextForMailingList(list string) context.Context {
	mc.Mutex.Lock()
	defer mc.Mutex.Unlock()

	cc, ok := mc.CancelMap[list]
	if ok {
		return cc.Ctx
	} else {
		newCtx, cancel := context.WithCancel(context.Background())
		mc.CancelMap[list] = CancellableContext{Ctx: newCtx, Cancel: cancel}
		return newCtx
	}
}

func (mc *MailCanceller) CancelMailingList(list string) {
	mc.Mutex.Lock()
	defer mc.Mutex.Unlock()

	cc, ok := mc.CancelMap[list]
	if !ok {
		return
	}
	cc.Cancel()

	delete(mc.CancelMap, list)
}

func (mc *MailCanceller) ListHasContext(list string) bool {
	mc.Mutex.Lock()
	defer mc.Mutex.Unlock()

	_, ok := mc.CancelMap[list]
	return ok
}
