package protocol

import (
	"bytes"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/AkvicorEdwards/glog"
)

const VERSION uint8 = 1

var ErrorReadCallbackIsNil = errors.New("read callback is nil")
var ErrorReaderIsNil = errors.New("reader is nil")
var ErrorWriterIsNil = errors.New("writer is nil")
var ErrorReaderIsKilled = errors.New("reader is killed")
var ErrorWriterIsKilled = errors.New("writer is killed")
var ErrorWriterQueueIsNil = errors.New("writer queue is nil")
var ErrorHeartbeatIsKilled = errors.New("heartbeat is killed")
var ErrorHeartbeatCallbackIsNil = errors.New("heartbeat callback is nil")
var ErrorDataSizeExceedsLimit = errors.New("data size exceeds limit")
var ErrorTimeout = errors.New("timeout")

const (
	statusRunning int32 = iota
	statusKilled
)

type Protocol struct {
	// 标记protocol
	tag string
	r   io.Reader
	w   io.Writer

	// protocol的状态
	status int32
	// 用于处理获取到的数据，每个package中的数据都会完整的保存在data中
	readCallback func(data []byte)
	// 写入等待队列
	writeQueue *queue
	// 当前protocol正在运行的协程数量
	runningRoutines int32

	// 心跳信号，同时也是心跳响应信号
	heartbeatSig chan uint8
	// 心跳请求信号，收到此信号必须回复对方
	heartbeatSigReq chan uint8
	// 发送心跳请求的间隔
	heartbeatInterval uint32
	// 接收心跳请求的超时时间
	heartbeatTimeout uint32
	// 心跳请求超时后的处理函数
	heartbeatTimeoutCallback func(p *Protocol) bool
	// 上次发送心跳的时间
	heartbeatLastSend int64
	// 上次收到心跳的时间
	heartbeatLastReceived int64

	// status被标记为statusKilled时执行，可以用于关闭reader和writer
	killCallback func()
	// 在reader读取数据前，设置reader的读取截止时间
	setReadDeadline func()
	// 在writer读取数据前，设置writer的读取截止时间
	setWriteDeadline func()
}

// New 返回一个protocol实例
//
//	tag: 标签，用于区分protocol实例
//	r: 数据流的reader
//	w: 数据流的writer
//	writeQueueSize: 发送等待队列长度
//	readCallback: 用于处理获取到的数据，每个package中的数据都会完整的保存在data中
//	heartbeatTimeoutCallback: 心跳请求超时后的处理函数
//	setReadDeadline: 在reader读取数据前，设置reader的读取截止时间
//	setWriteDeadline: 在writer读取数据前，设置writer的读取截止时间
//	killCallback: status被标记为statusKilled时执行，可以用于关闭reader和writer
func New(tag string, r io.Reader, w io.Writer, writeQueueSize int, readCallback func(data []byte), heartbeatTimeoutCallback func(p *Protocol) bool, setReadDeadline, setWriteDeadline, killCallback func()) *Protocol {
	if r == nil {
		glog.Warning("[protocol.%s] reader is nil", tag)
		return nil
	}
	if w == nil {
		glog.Warning("[protocol.%s] writer is nil", tag)
		return nil
	}
	if writeQueueSize < 1 {
		glog.Trace("[protocol.%s] writeQueueSize is < 1, use 1", tag)
		writeQueueSize = 1
	}
	if readCallback == nil {
		glog.Trace("[protocol.%s] readCallback is nil, use defaultReadCallback", tag)
		readCallback = defaultReadCallback
	}
	if heartbeatTimeoutCallback == nil {
		glog.Trace("[protocol.%s] heartbeatTimeoutCallback is nil, use defaultHeartbeatTimeoutCallback", tag)
		heartbeatTimeoutCallback = defaultHeartbeatTimeoutCallback
	}
	if killCallback == nil {
		glog.Trace("[protocol.%s] killCallback is nil, use defaultKillCallback", tag)
		killCallback = defaultKillCallback
	}
	return &Protocol{
		tag:                      tag,
		r:                        r,
		w:                        w,
		status:                   statusRunning,
		readCallback:             readCallback,
		writeQueue:               newQueue(writeQueueSize),
		runningRoutines:          0,
		heartbeatSig:             make(chan uint8, 1),
		heartbeatSigReq:          make(chan uint8, 1),
		heartbeatInterval:        15,
		heartbeatTimeout:         30,
		heartbeatTimeoutCallback: heartbeatTimeoutCallback,
		heartbeatLastSend:        0,
		heartbeatLastReceived:    0,
		killCallback:             killCallback,
		setReadDeadline:          setReadDeadline,
		setWriteDeadline:         setWriteDeadline,
	}
}

func (p *Protocol) Connect(activeHeartbeatSignalSender bool) {
	go p.reader()
	go p.writer()
	go p.heartbeat()
	if activeHeartbeatSignalSender {
		go p.heartbeatSignalSender()
	}
}

func (p *Protocol) handlePackage(pkg *protocolPackage) {
	glog.Trace("[protocol.%s] handle package", p.tag)
	if pkg == nil {
		glog.Trace("[protocol.%s] package is nil", p.tag)
		return
	}
	if pkg.isEncrypted() {
		glog.Trace("[protocol.%s] package is encrypted, decrypt package", p.tag)
		pkg.decrypt()
	}
	if !pkg.checkHead() {
		glog.Trace("[protocol.%s] package head broken", p.tag)
		return
	}
	if (pkg.flag & flagHeartbeat) != 0 {
		glog.Trace("[protocol.%s] heartbeat signal in package", p.tag)
		p.heartbeatSig <- pkg.value
	}
	if (pkg.flag & flagHeartbeatRequest) != 0 {
		glog.Trace("[protocol.%s] heartbeat request signal in package", p.tag)
		p.heartbeatSigReq <- pkg.value
	}
	if !pkg.checkData() {
		glog.Trace("[protocol.%s] package data broken", p.tag)
		return
	}
	if pkg.dataSize == 0 {
		glog.Trace("[protocol.%s] package data empty", p.tag)
		return
	}
	glog.Trace("[protocol.%s] handle package successful", p.tag)
	p.readCallback(pkg.data)
}

// Reader 阻塞接收数据并提交给readCallback
func (p *Protocol) reader() {
	glog.Trace("[protocol.%s] reader enable", p.tag)
	if p.r == nil {
		glog.Warning("[protocol.%s] reader is not ready", p.tag)
		return
	}
	p.incRunningRoutine()
	defer p.decRunningRoutine()

	buffer := &bytes.Buffer{}
	buf := make([]byte, packageMaxSize)
	var err error
	var n int
	// 监听并接收数据
	for {
		if p.getStatus() == statusKilled {
			glog.Trace("[protocol.%s] reader is killed", p.tag)
			return
		}
		if p.setReadDeadline != nil {
			glog.Trace("[protocol.%s] reader set deadline", p.tag)
			p.setReadDeadline()
		}
		glog.Trace("[protocol.%s] reader wait read", p.tag)
		n, err = p.r.Read(buf)
		if err != nil {
			glog.Trace("[protocol.%s] read error %v", p.tag, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if n == 0 {
			glog.Trace("[protocol.%s] read empty", p.tag)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		n, err = buffer.Write(buf[:n])
		glog.Trace("[protocol.%s] write %d bytes, buffer already %d bytes, error is %v", p.tag, n, buffer.Len(), err)
		for buffer.Len() >= packageHeadSize {
			glog.Trace("[protocol.%s] complete package, buffer length %d", p.tag, buffer.Len())
			pkg, err := parsePackage(buffer)
			if errors.Is(err, ErrorPackageIncomplete) {
				glog.Trace("[protocol.%s] incomplete package, buffer length %d", p.tag, buffer.Len())
				break
			}
			if pkg != nil {
				glog.Trace("[protocol.%s] receive new package", p.tag)
				go p.handlePackage(pkg)
			}
		}
	}
}

// Writer 创建发送队列并监听待发送数据
func (p *Protocol) writer() {
	glog.Trace("[protocol.%s] writer enable", p.tag)
	if p.w == nil {
		glog.Warning("[protocol.%s] writer is not ready", p.tag)
		return
	}
	p.incRunningRoutine()
	defer p.decRunningRoutine()

	var err error
	var n int
	for {
		if p.getStatus() == statusKilled {
			glog.Trace("[protocol.%s] writer is killed", p.tag)
			return
		}
		glog.Trace("[protocol.%s] writer wait pop", p.tag)
		pkg := p.writeQueue.pop(int(p.GetHeartbeatInterval()))
		if pkg == nil {
			glog.Trace("[protocol.%s] writer pop timeout", p.tag)
			continue
		}
		if p.setWriteDeadline != nil {
			glog.Trace("[protocol.%s] writer set deadline", p.tag)
			p.setWriteDeadline()
		}
		glog.Trace("[protocol.%s] writer wait write", p.tag)
		n, err = p.w.Write(pkg.Bytes().Bytes())
		glog.Trace("[protocol.%s] write %d bytes, error is %v", p.tag, n, err)
		if err != nil {
			glog.Trace("[protocol.%s] send package failed, re-push package", p.tag)
			time.Sleep(time.Second)
			for !p.writeQueue.push(pkg, int(p.GetHeartbeatInterval())) {
				if p.getStatus() == statusKilled {
					glog.Trace("[protocol.%s] writer is killed", p.tag)
					return
				}
			}
		}
	}
}

// Write 发送数据
func (p *Protocol) Write(data []byte) error {
	glog.Trace("[protocol.%s] write", p.tag)
	if len(data) > dataMaxSize {
		glog.Info("[protocol.%s] maximum supported data size exceeded", p.tag)
		return ErrorDataSizeExceedsLimit
	}
	pkg := newPackage(0, encryptNone, 0, data)
	for {
		if p.getStatus() == statusKilled {
			glog.Info("[protocol.%s] protocol is killed", p.tag)
			return ErrorWriterIsKilled
		}
		if p.writeQueue.push(pkg, int(p.GetHeartbeatInterval())) {
			return nil
		}
	}
}

// heartbeat 心跳服务
//
//	heartbeatTimeout: 被动接收心跳信号的超时时间(s)，最小为3s，传入参数小于3时使用默认值30
//	heartbeatTimeoutCallback: 没有按时收到心跳信号时调用，返回true继续等待，返回false退出
func (p *Protocol) heartbeat() {
	glog.Trace("[protocol.%s] heartbeat enable", p.tag)
	p.incRunningRoutine()
	defer p.decRunningRoutine()

	for {
		select {
		case <-time.After(time.Duration(p.GetHeartbeatTimeout()) * time.Second):
			glog.Trace("[protocol.%s] heartbeat timeout", p.tag)
			if p.getStatus() == statusKilled {
				glog.Trace("[protocol.%s] heartbeat is killed", p.tag)
				return
			}
			if !p.heartbeatTimeoutCallback(p) {
				glog.Trace("[protocol.%s] heartbeat is killed, set status killed", p.tag)
				p.setStatus(statusKilled)
				return
			}
		case val := <-p.heartbeatSigReq:
			glog.Trace("[protocol.%s] heartbeat request signal received", p.tag)
			p.setHeartbeatLastReceived()
			p.sendHeartbeatSignal(false)
			if val != 0 {
				p.SetHeartbeatTimeout(val)
			}
		case val := <-p.heartbeatSig:
			glog.Trace("[protocol.%s] heartbeat signal received", p.tag)
			p.setHeartbeatLastReceived()
			if val != 0 {
				p.SetHeartbeatTimeout(val)
			}
		}
		if p.getStatus() == statusKilled {
			glog.Trace("[protocol.%s] heartbeat is killed", p.tag)
			return
		}
	}
}

// heartbeatSignalSender 主动触发心跳
//
//	heartbeatInterval: 主动发送心跳信号的间隔时间(s)，最小为3s，传入参数小于3时使用默认值3
func (p *Protocol) heartbeatSignalSender() {
	p.incRunningRoutine()
	defer p.decRunningRoutine()
	for {
		if p.getStatus() == statusKilled {
			glog.Trace("[protocol.%s] heartbeat signal sender is killed", p.tag)
			return
		}
		p.sendHeartbeatSignal(true)
		time.Sleep(time.Duration(p.GetHeartbeatInterval()) * time.Second)
	}
}

func (p *Protocol) sendHeartbeatSignal(isReq bool) {
	glog.Trace("[protocol.%s] send heartbeat signal", p.tag)
	var pkg *protocolPackage
	if isReq {
		pkg = newPackage(flagHeartbeatRequest, encryptNone, 0, nil)
	} else {
		pkg = newPackage(flagHeartbeat, encryptNone, 0, nil)
	}
	for !p.writeQueue.push(pkg, int(p.GetHeartbeatInterval())) {
		if p.getStatus() == statusKilled {
			glog.Info("[protocol.%s] protocol is killed", p.tag)
			return
		}
	}
	p.setHeartbeatLastSend()
}

func (p *Protocol) setStatus(status int32) {
	glog.Trace("[protocol.%s] set status %d", p.tag, status)
	if status == statusKilled {
		p.killCallback()
	}
	atomic.StoreInt32(&p.status, status)
}

func (p *Protocol) getStatus() int32 {
	glog.Trace("[protocol.%s] get status", p.tag)
	return atomic.LoadInt32(&p.status)
}

func (p *Protocol) SetHeartbeatInterval(interval uint8) {
	if interval < 3 {
		glog.Trace("[protocol.%s] heartbeatInterval is < 3, use 3", p.tag)
		interval = 3
	}
	atomic.StoreUint32(&p.heartbeatInterval, uint32(interval))
}

func (p *Protocol) GetHeartbeatInterval() uint8 {
	return uint8(atomic.LoadUint32(&p.heartbeatInterval))
}

func (p *Protocol) SetHeartbeatTimeout(timeout uint8) {
	if timeout < 6 {
		glog.Trace("[protocol.%s] heartbeatTimeout is < 6, use 6", p.tag)
		timeout = 6
	}
	atomic.StoreUint32(&p.heartbeatTimeout, uint32(timeout))
}

func (p *Protocol) GetHeartbeatTimeout() uint8 {
	return uint8(atomic.LoadUint32(&p.heartbeatTimeout))
}

func (p *Protocol) setHeartbeatLastReceived() {
	atomic.StoreInt64(&p.heartbeatLastReceived, time.Now().Unix())
}

func (p *Protocol) GetHeartbeatLastReceived() int64 {
	return atomic.LoadInt64(&p.heartbeatLastReceived)
}

func (p *Protocol) setHeartbeatLastSend() {
	atomic.StoreInt64(&p.heartbeatLastSend, time.Now().Unix())
}

func (p *Protocol) GetHeartbeatLastSend() int64 {
	return atomic.LoadInt64(&p.heartbeatLastSend)
}

func (p *Protocol) incRunningRoutine() {
	atomic.AddInt32(&p.runningRoutines, 1)
}

func (p *Protocol) decRunningRoutine() {
	atomic.AddInt32(&p.runningRoutines, -1)
}

func (p *Protocol) GetRunningRoutine() int32 {
	return atomic.LoadInt32(&p.runningRoutines)
}

func (p *Protocol) WaitKilled(timeout int) error {
	out := time.After(time.Duration(timeout) * time.Second)
	for {
		select {
		case <-out:
			return ErrorTimeout
		case <-time.After(time.Second):
			if p.GetRunningRoutine() <= 0 {
				return nil
			}
		}
	}
}

func (p *Protocol) GetTag() string {
	return p.tag
}

func (p *Protocol) SetTag(tag string) {
	p.tag = tag
}

func (p *Protocol) Kill() {
	p.setStatus(statusKilled)
}

func defaultReadCallback(data []byte) {
	glog.Trace("[protocol] default read callback %x", data)
}

func defaultHeartbeatTimeoutCallback(*Protocol) bool {
	glog.Trace("[protocol] default heartbeat timeout callback")
	return true
}

func defaultKillCallback() {
	glog.Trace("[protocol] default kill callback")
}

func GetDataMaxSize() int {
	return dataMaxSize
}

func CalculateTheNumberOfPackages(size int64) int64 {
	res := size / dataMaxSize
	if size%dataMaxSize != 0 {
		res += 1
	}
	return res
}
