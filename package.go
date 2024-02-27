package protocol

import (
	"bytes"
	"errors"

	"github.com/AkvicorEdwards/glog"
	"github.com/AkvicorEdwards/util"
)

// package的起始标志
var prefix = [headLengthPrefix]uint8{0xff, 0x07, 0x55, 0x00}

var ErrorPackageIncomplete = errors.New("package incomplete")
var ErrorUnsupportedVersion = errors.New("unsupported version")
var ErrorWrongPrefix = errors.New("prefix does not match")
var ErrorBrokenHead = errors.New("head crc32 checksum does not match")
var ErrorBrokenData = errors.New("data crc32 checksum does not match")

// head中各部分的长度
const (
	headLengthPrefix        = 4
	headLengthVersion       = 1
	headLengthCRC32Checksum = 4
	headLengthFlag          = 1
	headLengthEncryptMethod = 1
	headLengthCustomValue   = 1
	headLengthDataSize      = 4
	headLengthDataCrc32     = 4
)

// head中各部分的偏移
const (
	headOffsetPrefix        = 0
	headOffsetVersion       = headOffsetPrefix + headLengthPrefix
	headOffsetCRC32Checksum = headOffsetVersion + headLengthVersion
	headOffsetFlag          = headOffsetCRC32Checksum + headLengthCRC32Checksum
	headOffsetEncryptMethod = headOffsetFlag + headLengthFlag
	headOffsetCustomValue   = headOffsetEncryptMethod + headLengthEncryptMethod
	headOffsetDataSize      = headOffsetCustomValue + headLengthCustomValue
	headOffsetDataCrc32     = headOffsetDataSize + headLengthDataSize
	headOffsetData          = headOffsetDataCrc32 + headLengthDataCrc32
)

// 计算head的crc32时起始偏移
const headOffsetNeedCheck = headOffsetCRC32Checksum + headLengthCRC32Checksum

// data的加密方式
const (
	encryptNone uint8 = iota
)

// flag标志位
const (
	// 普通心跳信号，心跳响应信号
	flagHeartbeat uint8 = 1 << iota
	// 心跳请求信号，接收方必须回复flagHeartbeat
	flagHeartbeatRequest
)

// package的head的大小 (byte)
const packageHeadSize = headOffsetData

// package的最大size (byte)
const packageMaxSize = 4096

// package的data的最大size (byte)
const dataMaxSize = packageMaxSize - packageHeadSize

type protocolPackage struct {
	prefix        [headLengthPrefix]byte // 4 byte 0xff 0x55
	version       uint8                  // 1 byte protocol version
	crc32         uint32                 // 4 byte head crc32 checksum (BigEndian)
	flag          uint8                  // 1 byte flag
	encryptMethod uint8                  // 1 byte encrypted method
	value         uint8                  // 1 byte custom value (for heartbeat)
	dataSize      uint32                 // 4 byte curSize of data (BigEndian)
	dataCrc32     uint32                 // 4 byte crc32 of data (BigEndian)
	data          []byte                 // ? byte data
}

// 创建新package, 所有新package的创建都必须通过此方法
func newPackage(flag uint8, encrypt uint8, value uint8, data []byte) *protocolPackage {
	pkg := &protocolPackage{
		prefix:        prefix,
		version:       VERSION,
		crc32:         0,
		flag:          flag,
		encryptMethod: encryptNone,
		value:         value,
		dataSize:      uint32(len(data)),
		dataCrc32:     0,
		data:          data,
	}
	pkg.generateDataCheck()
	pkg.generateHeadCheck()
	pkg.encrypt(encrypt)
	return pkg
}

func (p *protocolPackage) Bytes() *bytes.Buffer {
	buf := &bytes.Buffer{}
	// prefix
	buf.Write(p.prefix[:])
	// version
	buf.WriteByte(p.version)
	// crc32
	buf.Write(util.UInt32ToBytesSlice(p.crc32))
	// flag
	buf.WriteByte(p.flag)
	// encrypt method
	buf.WriteByte(p.encryptMethod)
	// value
	buf.WriteByte(p.value)
	// data curSize
	buf.Write(util.UInt32ToBytesSlice(p.dataSize))
	// data crc32
	buf.Write(util.UInt32ToBytesSlice(p.dataCrc32))
	// data
	buf.Write(p.data)
	return buf
}

// 将head中需要校验的数据拼接起来
func (p *protocolPackage) headNeedCheckBytes() *bytes.Buffer {
	buf := &bytes.Buffer{}
	buf.WriteByte(p.flag)
	buf.WriteByte(p.encryptMethod)
	buf.WriteByte(p.value)
	buf.Write(util.UInt32ToBytesSlice(p.dataSize))
	buf.Write(util.UInt32ToBytesSlice(p.dataCrc32))
	return buf
}

// 生成head的crc32
func (p *protocolPackage) generateHeadCheck() {
	p.crc32 = util.NewCRC32().FromBytes(p.headNeedCheckBytes().Bytes()).Value()
	glog.Trace("[protocol_package] head crc32 is %d", p.crc32)
}

// 校验head的crc32
func (p *protocolPackage) checkHead() bool {
	return p.crc32 == util.NewCRC32().FromBytes(p.headNeedCheckBytes().Bytes()).Value()
}

// 生成data的crc32
func (p *protocolPackage) generateDataCheck() {
	p.dataCrc32 = util.NewCRC32().FromBytes(p.data).Value()
	glog.Trace("[protocol_package] data crc32 is %d", p.dataCrc32)
}

// 校验data的crc32
func (p *protocolPackage) checkData() bool {
	if int(p.dataSize) != len(p.data) {
		glog.Trace("[protocol_package] pkg.dataSize != len(pkg.data)")
		return false
	}
	return p.dataCrc32 == util.NewCRC32().FromBytes(p.data).Value()
}

// encrypt 加密data
func (p *protocolPackage) encrypt(method uint8) {
	if p.encryptMethod == method {
		glog.Trace("[protocol_package] is already encrypted [%d]", method)
		return // 已经加密
	}
	if p.encryptMethod != encryptNone {
		glog.Trace("[protocol_package] encrypt with other method got [%d] need encryptNone[%d]", p.encryptMethod, encryptNone)
		return // 已经通过其他方式加密
	}
	glog.Warning("[protocol_package] unknown encrypt method")
}

// decrypt 解密data
func (p *protocolPackage) decrypt() {
	if !p.isEncrypted() {
		glog.Trace("[protocol_package] is not encrypted")
		return
	}
}

// isEncrypted 是否已经加密
func (p *protocolPackage) isEncrypted() bool {
	return p.encryptMethod != encryptNone
}

// parsePackage 从buf中读取一个package
//
//	如果协议头标志(prefix)不匹配，删除buf中除第一个字符外，下一个prefix1到buf开头的所有数据
//	如果协议头标志(prefix)匹配，不断从buf中取出数据，填充到package结构体
//	如果buf中的数据出错，无法正确提取package, 则返回(nil,true), 且已从buf中提取的数据不会退回buf
func parsePackage(buf *bytes.Buffer) (*protocolPackage, error) {
	// 判断package的版本
	// 暂时只处理VERSION版本的package
	if buf.Len() < headOffsetVersion+headLengthVersion {
		glog.Trace("[protocol_package] incomplete version information, need %d got %d", headOffsetVersion+headLengthVersion, buf.Len())
		return nil, ErrorPackageIncomplete
	}
	if buf.Bytes()[headOffsetVersion] != VERSION {
		glog.Trace("[protocol_package] unsupported version need %d got %d", VERSION, buf.Bytes()[headOffsetVersion])
		nextPackageHead(buf)
		return nil, ErrorUnsupportedVersion
	}
	// 开始判断是否为package并提取package
	if buf.Len() < packageHeadSize {
		glog.Trace("[protocol_package] incomplete head, need %d got %d", packageHeadSize, buf.Len())
		return nil, ErrorPackageIncomplete
	}
	head := make([]byte, packageHeadSize)
	copy(head, buf.Bytes()[:packageHeadSize])
	// 协议头标志不匹配，删除未知数据，寻找下一个package起始位置
	if !bytes.Equal(prefix[:], head[headOffsetPrefix:headOffsetPrefix+headLengthPrefix]) {
		glog.Trace("[protocol_package] prefix does not match, need %v got %v", prefix, head[headOffsetPrefix:headOffsetPrefix+headLengthPrefix])
		nextPackageHead(buf)
		return nil, ErrorWrongPrefix
	}
	// 检查head是否完整，删除未知数据，寻找下一个package起始位置
	headChecksum := util.BytesSliceToUInt32(head[headOffsetCRC32Checksum : headOffsetCRC32Checksum+headLengthCRC32Checksum])
	headCrc32 := util.NewCRC32().FromBytes(head[headOffsetNeedCheck:]).Value()
	if headChecksum != headCrc32 {
		glog.Trace("[protocol_package] head crc32 checksum does not match, need %d got %d", headChecksum, headCrc32)
		nextPackageHead(buf)
		return nil, ErrorBrokenHead
	}
	// 检查package是否完整，不完整则等待
	packageDataSize := util.BytesSliceToUInt32(head[headOffsetDataSize : headOffsetDataSize+headLengthDataSize])
	if packageHeadSize+int(packageDataSize) > buf.Len() {
		glog.Trace("[protocol_package] incomplete data, need %d got %d", packageHeadSize+packageDataSize, buf.Len())
		return nil, ErrorPackageIncomplete
	}
	// package完整
	pkg := &protocolPackage{}
	_, _ = buf.Read(make([]byte, packageHeadSize))
	// prefix
	copy(pkg.prefix[:], head[headOffsetPrefix:headOffsetPrefix+headLengthPrefix])
	// crc32
	pkg.version = head[headOffsetVersion]
	// crc32
	pkg.crc32 = headCrc32
	// flag
	pkg.flag = head[headOffsetFlag]
	// encrypt method
	pkg.encryptMethod = head[headOffsetEncryptMethod]
	// value
	pkg.value = head[headOffsetCustomValue]
	// data curSize
	pkg.dataSize = packageDataSize
	// data crc32
	pkg.dataCrc32 = util.BytesSliceToUInt32(head[headOffsetDataCrc32 : headOffsetDataCrc32+headLengthDataCrc32])
	// data
	pkg.data = make([]byte, pkg.dataSize)
	_, _ = buf.Read(pkg.data)
	dataCrc32 := util.NewCRC32().FromBytes(pkg.data).Value()
	if pkg.dataCrc32 != dataCrc32 {
		glog.Trace("[protocol_package] data crc32 checksum does not match, need %d got %d", pkg.dataCrc32, dataCrc32)
		nextPackageHead(buf)
		return nil, ErrorBrokenData
	}
	return pkg, nil
}

// 删除掉buf中第一个byte, 并将buf中的起始位置调整到与prefix[0]相同的下一个元素的位置
func nextPackageHead(buf *bytes.Buffer) {
	var err error
	_, _ = buf.ReadByte()
	_, err = buf.ReadBytes(prefix[0]) // 只搜索与prefix[0]相同元素，防止prefix[0]出现在buf末尾
	if err == nil {                   // 找到下一个协议头标志，把删掉的prefix回退到buffer
		_ = buf.UnreadByte()
		glog.Trace("[protocol_package] prefix does not match, prefix[0] found, trim buf, buf length [%d]", buf.Len())
	} else { // 找不到下一个协议头标志，清空buffer
		buf.Reset()
		glog.Trace("[protocol_package] prefix does not match, prefix[0] not found, reset buf, buf length [%d]", buf.Len())
	}
}
