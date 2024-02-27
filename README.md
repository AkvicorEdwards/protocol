# Protocol

一个简易的数据传输协议，保证在流传输协议下，每次Write的数据对方能完整(不多&不少)的接收和处理

```go
import "github.com/AkvicorEdwards/protocol"
```

样例请参照`protocol_test.go`中的示例，示例中包含了对tcp的封装

```go
// 创建protocol封装
New(tag string, r io.Reader, w io.Writer, writeQueueSize int, readCallback func(data []byte), heartbeatTimeoutCallback func() bool, setReadDeadline, setWriteDeadline, killCallback func()) *Protocol
// 启动传输，通过参数确定是否是心跳服务端（主动发出心跳信号一方）
//    如果传输双方均为发出方，不会影响正常服务，但会产生不必要的心跳
Connect(bool)
// 发送数据（注意数据长度有限制
Write([]byte{0x11, 0x22, 0x33})
// 设置心跳请求间隔(单位秒)，用于主动发出心跳的一方
SetHeartbeatInterval(interval uint8)
GetHeartbeatInterval() uint8
// 设置心跳超时时间(单位秒)
SetHeartbeatTimeout(timeout uint8)
GetHeartbeatTimeout() uint8
// 获取上一次收到心跳的时间
GetHeartbeatLastReceived()
// 获取上一次发送心跳的时间
GetHeartbeatLastSend()
// 获取正在运行的协程的数量
GetRunningRoutine() int32
// 等待Protocol关闭
WaitKilled(timeout int) error
// 获取tag
GetTag() string
SetTag(tag string)
// 关闭Protocol
Kill()

// 设置log模式
SetLogProd(isProd bool)
SetLogMask(mask uint32)
SetLogFlag(f uint32)

// Protocol版本号，不同版本存在不兼容的可能性
protocol.VERSION
// 获取每次Write的最大数据长度
protocol.GetDataMaxSize() int
// 计算传入的size需要多少次Write才能发送
protocol.CalculateTheNumberOfPackages(size int64) int64
```
