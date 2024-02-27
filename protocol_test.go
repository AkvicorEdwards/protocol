package protocol

import (
	"errors"
	"fmt"
	"github.com/AkvicorEdwards/glog"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

var serverClosed = make(chan bool, 1)
var clientClosed = make(chan bool, 1)

func TestProtocol(t *testing.T) {
	SetLogMask(MaskFATAL | MaskERROR | MaskWARNING | MaskINFO)
	go testServer(t)
	time.Sleep(time.Second)
	go testClient(t)

	<-clientClosed
	<-serverClosed
}

func testServer(t *testing.T) {
	defer func() {
		serverClosed <- true
	}()
	listen, err := net.Listen("tcp", "0.0.0.0:9999")
	if err != nil {
		glog.Error("[server] Listen() failed, err: %s", err)
		return
	}
	glog.Info("[server] Listen 0.0.0.0:9999")
	for {
		conn, err := listen.Accept() // 监听客户端的连接请求
		if err != nil {
			glog.Error("[server] Accept() failed, err: %s", err)
			continue
		}
		glog.Info("[server] Accept %s %s", conn.LocalAddr().String(), conn.RemoteAddr().String())
		var Index uint32 = 0
		protServer := New("server", conn, conn, 8, func(data []byte) {
			// 处理获取到的数据
			fmt.Printf("[server] received [%s]\n", string(data))
			atomic.AddUint32(&Index, 1)
			ans := fmt.Sprintf("client msg %d", atomic.LoadUint32(&Index))
			if ans != string(data) {
				t.Errorf("test client error need %s got %s", ans, string(data))
			}
		}, func(p *Protocol) bool {
			// protocol还在运行，但心跳超时
			fmt.Println("[server] heartbeat timeout")
			return false
		}, func() {
			// 每次conn.Read前运行
			_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		}, func() {
			// 每次conn.Write前运行
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		}, func() {
			// protocol状态更改为killed时运行
			_ = conn.Close()
		})
		protServer.SetHeartbeatInterval(3)
		protServer.SetHeartbeatTimeout(10)
		protServer.Connect(true)

		i := 0
		for {
			time.Sleep(5 * time.Second)
			i++
			msg := fmt.Sprintf("server msg %d", i)
			fmt.Printf("[server] send [%s]\n", msg)
			err = protServer.Write([]byte(msg))
			if err != nil {
				if !errors.Is(err, ErrorWriterIsKilled) {
					glog.Warning("[server] failed to write %v", err)
				} else {
					if protServer.GetHeartbeatLastSend() == 0 {
						t.Error("server.GetHeartbeatLastSend is zero")
					}
					if protServer.GetHeartbeatLastReceived() == 0 {
						t.Error("server.GetHeartbeatLastReceived is zero")
					}
					glog.Info("wait server killed")
					err = protServer.WaitKilled(60)
					if err != nil {
						t.Errorf("server killed failed [%d]", protServer.GetRunningRoutine())
					}
					glog.Info("wait server killed [%d]", protServer.GetRunningRoutine())
				}
				return
			}
		}
	}
}

func testClient(t *testing.T) {
	defer func() {
		clientClosed <- true
	}()
	conn, err := net.Dial("tcp", "127.0.0.1:9999")
	if err != nil {
		glog.Error("[client] Dial() failed, err: %s", err)
		return
	}
	glog.Info("[client] Connected")

	var Index uint32 = 0
	protClient := New("client", conn, conn, 8, func(data []byte) {
		fmt.Printf("[client] received [%s]\n", string(data))
		atomic.AddUint32(&Index, 1)
		ans := fmt.Sprintf("server msg %d", atomic.LoadUint32(&Index))
		if ans != string(data) {
			t.Errorf("test client error need %s got %s", ans, string(data))
		}
	}, func(*Protocol) bool {
		fmt.Println("[client] heartbeat timeout")
		return true
	}, func() {
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	}, func() {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	}, func() {
		_ = conn.Close()
	})
	protClient.SetHeartbeatInterval(3)
	protClient.SetHeartbeatTimeout(10)
	protClient.Connect(false)
	time.Sleep(1 * time.Second)
	i := 0
	for {
		time.Sleep(5 * time.Second)
		i++
		msg := fmt.Sprintf("client msg %d", i)
		fmt.Printf("[client] send [%s]\n", msg)
		err = protClient.Write([]byte(msg))
		if err != nil {
			if !errors.Is(err, ErrorWriterIsKilled) {
				glog.Warning("[client] failed to write %v", err)
			}
			return
		}
		if i == 6 {
			if protClient.GetHeartbeatLastSend() == 0 {
				t.Error("client.GetHeartbeatLastSend is zero")
			}
			if protClient.GetHeartbeatLastReceived() == 0 {
				t.Error("client.GetHeartbeatLastReceived is zero")
			}

			glog.Info("kill client")
			protClient.Kill()
			glog.Info("wait client killed")
			err := protClient.WaitKilled(60)
			if err != nil {
				t.Errorf("kill client failed [%d]", protClient.GetRunningRoutine())
			}
			glog.Info("wait client killed [%d]", protClient.GetRunningRoutine())
			return
		}
	}
}
