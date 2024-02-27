package protocol

import (
	"github.com/AkvicorEdwards/glog"
)

const (
	MaskUNKNOWN = glog.MaskUNKNOWN
	MaskDEBUG   = glog.MaskDEBUG
	MaskTRACE   = glog.MaskTRACE
	MaskINFO    = glog.MaskINFO
	MaskWARNING = glog.MaskWARNING
	MaskERROR   = glog.MaskERROR
	MaskFATAL   = glog.MaskFATAL

	MaskStd = glog.MaskStd
	MaskAll = glog.MaskAll

	MaskDev  = MaskFATAL | MaskERROR | MaskWARNING | MaskINFO | MaskTRACE | MaskDEBUG | MaskUNKNOWN
	MaskProd = MaskFATAL | MaskERROR | MaskWARNING
)

const (
	FlagDate      = glog.FlagDate
	FlagTime      = glog.FlagTime
	FlagLongFile  = glog.FlagLongFile
	FlagShortFile = glog.FlagShortFile
	FlagFunc      = glog.FlagFunc
	FlagPrefix    = glog.FlagPrefix
	FlagSuffix    = glog.FlagSuffix

	FlagStd = glog.FlagStd
	FlagAll = glog.FlagAll

	FlagDev  = FlagDate | FlagTime | FlagShortFile | FlagFunc | FlagPrefix | FlagSuffix
	FlagProd = FlagDate | FlagTime | FlagPrefix
)

func SetLogProd(isProd bool) {
	if isProd {
		glog.SetMask(MaskProd)
		glog.SetFlag(FlagProd)
	} else {
		glog.SetMask(MaskDev)
		glog.SetFlag(FlagDev)
	}
}

func SetLogMask(mask uint32) {
	glog.SetMask(mask)
}

func SetLogFlag(f uint32) {
	glog.SetFlag(f)
}

func init() {
	glog.SetMask(MaskProd)
	glog.SetFlag(FlagProd)
}
