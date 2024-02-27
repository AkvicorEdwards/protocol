package protocol

import (
	"sync"
	"sync/atomic"
	"time"
)

type queue struct {
	poolStart chan bool
	poolEnd   chan bool
	pushLock  sync.Mutex
	popLock   sync.Mutex
	maxSize   int
	curSize   int32
	wIndex    int
	rIndex    int
	queue     []*protocolPackage
}

func newQueue(size int) *queue {
	if size < 1 {
		size = 1
	}
	return &queue{
		// Start和End信号池用于保证push和pop操作不会互相干扰
		// 每次Push和Pop操作后，两个信号池中的信号数量都会保持一致
		poolStart: make(chan bool, size),
		poolEnd:   make(chan bool, size),
		// 保证push操作完整性
		pushLock: sync.Mutex{},
		// 保证pop操作完整性
		popLock: sync.Mutex{},
		// 队列中元素最大数量
		maxSize: size,
		// 队列当前元素数量
		curSize: 0,
		// push指针
		wIndex: 0,
		// pop指针
		rIndex: 0,
		// 元素数组
		queue: make([]*protocolPackage, size),
	}
}

func (q *queue) push(item *protocolPackage, timeout int) (res bool) {
	q.pushLock.Lock()
	defer func() {
		// push成功后队列大小+1
		atomic.AddInt32(&q.curSize, 1)
		q.pushLock.Unlock()
		if res {
			// 向End信号池发送一个信号，表示完成此次push
			q.poolEnd <- true
		}
	}()
	// 操作成功代表队列不满，向Start信号池发送一个信号，表示开始push
	if timeout > 0 {
		select {
		case q.poolStart <- true:
		case <-time.After(time.Duration(timeout) * time.Second):
			res = false
			return
		}
	} else {
		q.poolStart <- true
	}

	q.queue[q.wIndex] = item

	q.wIndex++
	if q.wIndex >= q.maxSize {
		q.wIndex = 0
	}
	res = true
	return
}

func (q *queue) pop(timeout int) (item *protocolPackage) {
	q.popLock.Lock()
	defer func() {
		// pop成功后队列大小-1
		atomic.AddInt32(&q.curSize, -1)
		q.popLock.Unlock()
		if item != nil {
			// 当前元素已经成功取出，释放当前位置
			<-q.poolStart
		}
	}()
	// 操作成功代表队列非空，只有End信号池中有信号，才能保证有完整的元素在队列中
	if timeout > 0 {
		select {
		case <-q.poolEnd:
		case <-time.After(time.Duration(timeout) * time.Second):
			item = nil
			return
		}
	} else {
		<-q.poolEnd
	}

	item = q.queue[q.rIndex]

	q.queue[q.rIndex] = nil

	q.rIndex++
	if q.rIndex >= q.maxSize {
		q.rIndex = 0
	}
	return
}

func (q *queue) size() int32 {
	return atomic.LoadInt32(&q.curSize)
}

func (q *queue) isEmpty() bool {
	return atomic.LoadInt32(&q.curSize) == 0
}
