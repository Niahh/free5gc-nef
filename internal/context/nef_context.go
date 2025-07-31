package context

import (
	"context"
	"fmt"
	"github.com/free5gc/nef/pkg/factory"
	"strconv"
	"strings"
	"sync"

	"github.com/free5gc/nef/internal/logger"
	"github.com/free5gc/openapi/models"
	"github.com/free5gc/openapi/oauth"
	"github.com/google/uuid"
)

var nefContext NefContext

type NefContext struct {
	name           string                                                 // Name given to this NEF instance
	nfInstID       string                                                 // NetworkFunction Instance ID of the NEF
	nfServices     map[models.ServiceName]models.NrfNfManagementNfService // nfservice that nef support
	uriScheme      models.UriScheme
	pcfPaUri       string
	udrDrUri       string
	nrfUri         string // Address of the NRF where the NEF will register and do its request
	nrfCertPem     string // Certificate of the NRF used if OAuth2 is required
	numCorreID     uint64
	OAuth2Required bool
	afs            map[string]*AfData
	bindingIPv4    string // Binding address for the SBI server where it will listen to
	sbiPort        int    // Port used by the NEF SBI server
	registerIPv4   string // Register ipv4 the NEF will provide to the NRF during the registration process

	mu sync.RWMutex
}

func init() {
	GetSelf().name = "nef"
	GetSelf().SetNfInstID(uuid.New().String())
	GetSelf().afs = make(map[string]*AfData)
	GetSelf().uriScheme = models.UriScheme_HTTPS
	GetSelf().nfServices = make(map[models.ServiceName]models.NrfNfManagementNfService)
}

func InitNefContext(context *NefContext) {
	config := factory.NefConfig
	logger.InitLog.Infof("nefConfig Info: GetVersion[%s]", config.GetVersion())
	configuration := config.Configuration
	if configuration.NefName != "" {
		context.SetName(configuration.NefName)
	}

	context.configureSbiContext(config)

	context.InitNFService(config.GetServiceNameList(), config.GetVersion())

	context.SetNrfUri(config.GetNrfUri())
	context.SetNrfCertPem(config.GetNrfCertPath())
}

func (c *NefContext) configureSbiContext(config *factory.Config) {
	if config.Configuration.Sbi != nil {
		c.SetUriScheme(config.GetSbiScheme())
		c.SetRegisterIPv4(config.GetSbiRegisterIP())
		c.SetSBIPort(config.GetSbiPort())
		c.SetBindingIPv4(config.GetSbiBindingIP())
	}
}

func (c *NefContext) GetName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.name
}

func (c *NefContext) SetName(newName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.name = newName
	logger.CtxLog.Infof("Set nf name: [%s]", c.name)
}

func (c *NefContext) GetNfInstID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nfInstID
}

func (c *NefContext) SetNfInstID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nfInstID = id
	logger.CtxLog.Infof("Set nfInstID: [%s]", c.nfInstID)
}

func (c *NefContext) GetNfServices() map[models.ServiceName]models.NrfNfManagementNfService {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nfServices
}

func (c *NefContext) SetNfServices(_nfServices map[models.ServiceName]models.NrfNfManagementNfService) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nfServices = _nfServices
	logger.CtxLog.Infof("Set nfServices: [%v]", c.nfServices)
}

func (c *NefContext) GetIPv4Uri() string {
	return fmt.Sprintf("%s://%s:%d", c.GetUriScheme(), c.GetRegisterIPv4(), c.GetSBIPort())
}

func (c *NefContext) InitNFService(serviceName []string, version string) {
	tmpVersion := strings.Split(version, ".")
	versionUri := "v" + tmpVersion[0]
	for index, nameString := range serviceName {
		name := models.ServiceName(nameString)
		nfServicesNames := c.GetNfServices()
		nfServicesNames[name] = models.NrfNfManagementNfService{
			ServiceInstanceId: strconv.Itoa(index),
			ServiceName:       name,
			Versions: []models.NfServiceVersion{
				{
					ApiFullVersion:  version,
					ApiVersionInUri: versionUri,
				},
			},
			Scheme:          c.GetUriScheme(),
			NfServiceStatus: models.NfServiceStatus_REGISTERED,
			ApiPrefix:       c.GetIPv4Uri(),
			IpEndPoints: []models.IpEndPoint{
				{
					Ipv4Address: c.GetRegisterIPv4(),
					Transport:   models.NrfNfManagementTransportProtocol_TCP,
					Port:        int32(c.GetSBIPort()),
				},
			},
		}
	}
}

func (c *NefContext) GetUriScheme() models.UriScheme {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.uriScheme
}

func (c *NefContext) SetUriScheme(newScheme string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uriScheme = models.UriScheme(newScheme)
	logger.CtxLog.Infof("Set uriScheme: [%s]", c.uriScheme)
}

func (c *NefContext) GetPcfPaUri() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pcfPaUri
}

func (c *NefContext) SetPcfPaUri(uri string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pcfPaUri = uri
	logger.CtxLog.Infof("Set pcfPaUri: [%s]", c.pcfPaUri)
}

func (c *NefContext) GetUdrDrUri() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.udrDrUri
}

func (c *NefContext) SetUdrDrUri(uri string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.udrDrUri = uri
	logger.CtxLog.Infof("Set udrDrUri: [%s]", c.udrDrUri)
}

func (c *NefContext) NewAf(afID string) *AfData {
	af := &AfData{
		AfID:     afID,
		Subs:     make(map[string]*AfSubscription),
		PfdTrans: make(map[string]*AfPfdTransaction),
		Log:      logger.CtxLog.WithField(logger.FieldAFID, fmt.Sprintf("AF:%s", afID)),
	}
	return af
}

func (c *NefContext) AddAf(af *AfData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.afs[af.AfID] = af
	af.Log.Infoln("AF is added")
}

func (c *NefContext) GetAf(afID string) *AfData {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.afs[afID]
}

func (c *NefContext) DeleteAf(afID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.afs, afID)
	logger.CtxLog.Infof("AF[%s] is deleted", afID)
}

func (c *NefContext) NewCorreID() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.numCorreID++
	return c.numCorreID
}

func (c *NefContext) ResetCorreID() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.numCorreID = 0
}

func (c *NefContext) IsAppIDExisted(appID string) (string, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, af := range c.afs {
		af.Mu.RLock()
		if transID, ok := af.IsAppIDExisted(appID); ok {
			defer af.Mu.RUnlock()
			return af.AfID, transID, true
		}
		af.Mu.RUnlock()
	}
	return "", "", false
}

func (c *NefContext) FindAfSub(CorrID string) (*AfData, *AfSubscription) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, af := range c.afs {
		af.Mu.RLock()
		for _, sub := range af.Subs {
			if sub.NotifCorreID == CorrID {
				defer af.Mu.RUnlock()
				return af, sub
			}
		}
		af.Mu.RUnlock()
	}
	return nil, nil
}

func (c *NefContext) GetBindingIPv4() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bindingIPv4
}

func (c *NefContext) SetBindingIPv4(_bindingIPv4 string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindingIPv4 = _bindingIPv4
	logger.CtxLog.Infof("Set BindingIPv4: [%s]", c.bindingIPv4)
}

func (c *NefContext) GetSBIPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sbiPort
}

func (c *NefContext) SetSBIPort(_sbiPort int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sbiPort = _sbiPort
	logger.CtxLog.Infof("Set SBIPort: [%d]", c.sbiPort)
}

func (c *NefContext) GetRegisterIPv4() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.registerIPv4
}

func (c *NefContext) SetRegisterIPv4(_registerIPv4 string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindingIPv4 = _registerIPv4
	logger.CtxLog.Infof("Set RegisterIPv4: [%s]", c.registerIPv4)
}

func (c *NefContext) GetNrfUri() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nrfUri
}

func (c *NefContext) SetNrfUri(_nrfUri string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nrfUri = _nrfUri
	logger.CtxLog.Infof("Set NRFUri: [%s]", c.nrfUri)
}

func (c *NefContext) GetNrfCertPem() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nrfCertPem
}

func (c *NefContext) SetNrfCertPem(_nrfCertPem string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nrfCertPem = _nrfCertPem
	logger.CtxLog.Infof("Set NRFCertPem: [%s]", c.nrfCertPem)
}

func (c *NefContext) GetTokenCtx(serviceName models.ServiceName, targetNF models.NrfNfManagementNfType) (
	context.Context, *models.ProblemDetails, error,
) {
	if !c.OAuth2Required {
		return context.TODO(), nil, nil
	}
	return oauth.GetTokenCtx(models.NrfNfManagementNfType_NEF, targetNF,
		c.nfInstID, c.GetNrfUri(), string(serviceName))
}

func GetSelf() *NefContext {
	return &nefContext
}
