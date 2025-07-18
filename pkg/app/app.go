package app

import (
	nef_context "github.com/free5gc/nef/internal/context"
	"github.com/free5gc/nef/pkg/factory"
)

type App interface {
	SetLogEnable(enable bool)
	SetLogLevel(level string)
	SetReportCaller(reportCaller bool)

	Start()
	Terminate()

	Context() *nef_context.NefContext
	Config() *factory.Config
}
