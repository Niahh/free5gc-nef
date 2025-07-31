/*
 * NEF Configuration Factory
 */

package factory

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/asaskevich/govalidator"
	"github.com/davecgh/go-spew/spew"
	"github.com/free5gc/nef/internal/logger"
	"github.com/free5gc/openapi/models"
)

const (
	ServiceTraffInflu  string = "3gpp-traffic-influence"
	ServicePfdMng      string = "3gpp-pfd-management"
	ServiceNefPfd      string = string(models.ServiceName_NNEF_PFDMANAGEMENT)
	ServiceNefOam      string = "nnef-oam"
	ServiceNefCallback string = "nnef-callback"
)

const (
	NefDefaultTLSKeyLogPath  = "./log/nefsslkey.log"
	NefDefaultCertPemPath    = "./cert/nef.pem"
	NefDefaultPrivateKeyPath = "./cert/nef.key"
	NefDefaultConfigPath     = "./config/nefcfg.yaml"
	NefExpectedConfigVersion = "1.0.1"
	NefSbiDefaultIPv4        = "127.0.0.5"
	NefSbiDefaultPort        = 8000
	NefSbiDefaultScheme      = "https"
	NefDefaultNrfUri         = "https://127.0.0.10:8000"
	TraffInfluResUriPrefix   = "/" + ServiceTraffInflu + "/v1"
	PfdMngResUriPrefix       = "/" + ServicePfdMng + "/v1"
	NefPfdMngResUriPrefix    = "/" + ServiceNefPfd + "/v1"
	NefOamResUriPrefix       = "/" + ServiceNefOam + "/v1"
	NefCallbackResUriPrefix  = "/" + ServiceNefCallback + "/v1"
)

type Config struct {
	Info          *Info          `yaml:"info" valid:"required"`
	Configuration *Configuration `yaml:"configuration" valid:"required"`
	Logger        *Logger        `yaml:"logger" valid:"required"`
	sync.RWMutex
}

func (c *Config) Validate() (bool, error) {
	govalidator.TagMap["scheme"] = func(str string) bool {
		return str == "https" || str == "http"
	}

	if configuration := c.Configuration; configuration != nil {
		if result, err := configuration.validate(); err != nil {
			return result, err
		}
	}

	result, err := govalidator.ValidateStruct(c)
	return result, appendInvalid(err)
}

type Info struct {
	Version     string `yaml:"version,omitempty" valid:"required, in(1.0.2)"`
	Description string `yaml:"description,omitempty" valid:"type(string)"`
}

type Configuration struct {
	NefName         string   `yaml:"nefName" valid:"required"`
	Sbi             *Sbi     `yaml:"sbi,omitempty" valid:"required"`
	NrfUri          string   `yaml:"nrfUri,omitempty" valid:"required"`
	NrfCertPem      string   `yaml:"nrfCertPem,omitempty" valid:"optional"`
	ServiceNameList []string `yaml:"serviceNameList,omitempty" valid:"required"`
}

type Logger struct {
	Enable       bool   `yaml:"enable" valid:"type(bool)"`
	Level        string `yaml:"level" valid:"required,in(trace|debug|info|warn|error|fatal|panic)"`
	ReportCaller bool   `yaml:"reportCaller" valid:"type(bool)"`
}

func (c *Configuration) validate() (bool, error) {
	if sbi := c.Sbi; sbi != nil {
		if result, err := sbi.validate(); err != nil {
			return result, err
		}
	}
	if c.ServiceNameList != nil {
		var errs govalidator.Errors
		services := []string{
			"3gpp-pfd-management",
			"3gpp-traffic-influence",
			"nnef-callback",
			"nnef-oam",
			"nnef-pfdmanagement",
		}
		for _, serviceName := range c.ServiceNameList {
			if !slices.Contains(services, serviceName) {
				err := fmt.Errorf("invalid ServiceNameList: %s,"+
					" value should be contained in %s", serviceName, strings.Join(services, " or "))
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return false, error(errs)
		}
	}
	result, err := govalidator.ValidateStruct(c)
	return result, appendInvalid(err)
}

type Sbi struct {
	Scheme       string `yaml:"scheme" valid:"scheme,required"`
	RegisterIPv4 string `yaml:"registerIPv4,omitempty" valid:"host,required"` // IP that is registered at NRF.
	// IPv6Addr  string `yaml:"ipv6Addr,omitempty"`
	BindingIPv4 string `yaml:"bindingIPv4,omitempty" valid:"host,required"` // IP used to run the server in the node.
	Port        int    `yaml:"port,omitempty" valid:"port,optional"`
	Tls         *Tls   `yaml:"tls,omitempty" valid:"optional"`
}

func (s *Sbi) validate() (bool, error) {
	govalidator.TagMap["scheme"] = govalidator.Validator(func(str string) bool {
		return str == "https" || str == "http"
	})

	if tls := s.Tls; tls != nil {
		if result, err := tls.validate(); err != nil {
			return result, err
		}
	}

	result, err := govalidator.ValidateStruct(s)
	return result, appendInvalid(err)
}

type Service struct {
	ServiceName string `yaml:"serviceName"`
	SuppFeat    string `yaml:"suppFeat,omitempty"`
}

type Tls struct {
	Pem string `yaml:"pem,omitempty" valid:"type(string),minstringlength(1),required"`
	Key string `yaml:"key,omitempty" valid:"type(string),minstringlength(1),required"`
}

func (t *Tls) validate() (bool, error) {
	result, err := govalidator.ValidateStruct(t)
	return result, err
}

func appendInvalid(err error) error {
	var errs govalidator.Errors
	if err == nil {
		return nil
	}
	es, ok := err.(govalidator.Errors)
	if ok {
		for _, e := range es.Errors() {
			errs = append(errs, fmt.Errorf("invalid %w", e))
		}
	} else {
		errs = append(errs, err)
	}
	return error(errs)
}

func (c *Config) Print() {
	c.RLock()
	defer c.RUnlock()

	spew.Config.Indent = "\t"
	str := spew.Sdump(c.Configuration)
	logger.CfgLog.Infof("==================================================")
	logger.CfgLog.Infof("%s", str)
	logger.CfgLog.Infof("==================================================")
}

func (c *Config) GetVersion() string {
	c.RLock()
	defer c.RUnlock()

	if c.Info.Version != "" {
		return c.Info.Version
	}
	return ""
}

func (c *Config) SetLogEnable(enable bool) {
	c.Lock()
	defer c.Unlock()

	if c.Logger == nil {
		logger.CfgLog.Warnf("Logger should not be nil")
		c.Logger = &Logger{
			Enable: enable,
			Level:  "info",
		}
	} else {
		c.Logger.Enable = enable
	}
}

func (c *Config) SetLogLevel(level string) {
	c.Lock()
	defer c.Unlock()

	if c.Logger == nil {
		logger.CfgLog.Warnf("Logger should not be nil")
		c.Logger = &Logger{
			Level: level,
		}
	} else {
		c.Logger.Level = level
	}
}

func (c *Config) SetLogReportCaller(reportCaller bool) {
	c.Lock()
	defer c.Unlock()

	if c.Logger == nil {
		logger.CfgLog.Warnf("Logger should not be nil")
		c.Logger = &Logger{
			Level:        "info",
			ReportCaller: reportCaller,
		}
	} else {
		c.Logger.ReportCaller = reportCaller
	}
}

func (c *Config) GetLogEnable() bool {
	c.RLock()
	defer c.RUnlock()
	if c.Logger == nil {
		logger.CfgLog.Warnf("Logger should not be nil")
		return false
	}
	return c.Logger.Enable
}

func (c *Config) GetLogLevel() string {
	c.RLock()
	defer c.RUnlock()
	if c.Logger == nil {
		logger.CfgLog.Warnf("Logger should not be nil")
		return "info"
	}
	return c.Logger.Level
}

func (c *Config) GetLogReportCaller() bool {
	c.RLock()
	defer c.RUnlock()
	if c.Logger == nil {
		logger.CfgLog.Warnf("Logger should not be nil")
		return false
	}
	return c.Logger.ReportCaller
}

func (c *Config) GetSbiScheme() string {
	c.RLock()
	defer c.RUnlock()

	if c.Configuration.Sbi.Scheme != "" {
		return c.Configuration.Sbi.Scheme
	}
	return NefSbiDefaultScheme
}

func (c *Config) GetSbiPort() int {
	c.RLock()
	defer c.RUnlock()

	if c.Configuration.Sbi.Port != 0 {
		return c.Configuration.Sbi.Port
	}
	return NefSbiDefaultPort
}

func (c *Config) GetSbiBindingIP() string {
	c.RLock()
	defer c.RUnlock()

	bindIP := "0.0.0.0"
	if c.Configuration.Sbi.BindingIPv4 != "" {
		if bindIP = os.Getenv(c.Configuration.Sbi.BindingIPv4); bindIP != "" {
			logger.CfgLog.Infof("Parsing ServerIPv4 [%s] from ENV Variable", bindIP)
		} else {
			bindIP = c.Configuration.Sbi.BindingIPv4
		}
	}
	return bindIP
}

func (c *Config) GetSbiBindingAddr() string {
	return c.GetSbiBindingIP() + ":" + strconv.Itoa(c.GetSbiPort())
}

func (c *Config) GetSbiRegisterIP() string {
	c.RLock()
	defer c.RUnlock()

	if c.Configuration.Sbi.RegisterIPv4 != "" {
		return c.Configuration.Sbi.RegisterIPv4
	}
	return NefSbiDefaultIPv4
}

func (c *Config) GetSbiRegisterAddr() string {
	return c.GetSbiRegisterIP() + ":" + strconv.Itoa(c.GetSbiPort())
}

func (c *Config) GetSbiUri() string {
	return c.GetSbiScheme() + "://" + c.GetSbiRegisterAddr()
}

func (c *Config) GetNrfUri() string {
	c.RLock()
	defer c.RUnlock()

	if c.Configuration.NrfUri != "" {
		return c.Configuration.NrfUri
	}
	return NefDefaultNrfUri
}

func (c *Config) GetNrfCertPath() string {
	c.RLock()
	defer c.RUnlock()

	if c.Configuration.NrfCertPem != "" {
		return c.Configuration.NrfCertPem
	}
	return "" // havn't setup in config
}

func (c *Config) GetServiceNameList() []string {
	c.RLock()
	defer c.RUnlock()
	if len(c.Configuration.ServiceNameList) > 0 {
		return c.Configuration.ServiceNameList
	}
	return nil
}

func (c *Config) GetCertPemPath() string {
	c.RLock()
	defer c.RUnlock()

	if c.Configuration.Sbi.Tls != nil {
		return c.Configuration.Sbi.Tls.Pem
	}
	return NefDefaultCertPemPath
}

func (c *Config) GetCertKeyPath() string {
	c.RLock()
	defer c.RUnlock()

	if c.Configuration.Sbi.Tls != nil {
		return c.Configuration.Sbi.Tls.Key
	}
	return NefDefaultPrivateKeyPath
}

func (c *Config) ServiceUri(name string) string {
	switch name {
	case ServiceTraffInflu:
		return c.GetSbiUri() + TraffInfluResUriPrefix
	case ServicePfdMng:
		return c.GetSbiUri() + PfdMngResUriPrefix
	case ServiceNefPfd:
		return c.GetSbiUri() + NefPfdMngResUriPrefix
	case ServiceNefOam:
		return c.GetSbiUri() + NefOamResUriPrefix
	case ServiceNefCallback:
		return c.GetSbiUri() + NefCallbackResUriPrefix
	default:
		return ""
	}
}
